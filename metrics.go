package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
)

func init() {
	// NOTE: this is to remove the registered pprof handlers from net/http/pprof init call.
	// The pprof handlers will be registered only if the pprof server is enabled.
	http.DefaultServeMux = http.NewServeMux()
}

const metricsServerDisabledAddr = ""
const pprofServerDisabledAddr = ""

type MetricsServerOptions struct {
	Logger  logr.Logger
	Addr    string
	Queryer sqlx.QueryerContext
}

func (opts *MetricsServerOptions) bindCLIFlags(fs *pflag.FlagSet) {
	fs.StringVar(
		&opts.Addr, "metrics-addr", ":8081",
		"metrics server listen address. Empty value means disabled.",
	)
}

func (opts *MetricsServerOptions) defaults() error {
	if opts.Logger.GetSink() == nil {
		opts.Logger = logr.Discard()
	}

	if opts.Addr != metricsServerDisabledAddr {
		if opts.Queryer == nil {
			return fmt.Errorf(".Queryer is required")
		}
	}

	return nil
}

type metricsServer struct {
	logger  logr.Logger
	server  *http.Server
	queryer sqlx.QueryerContext
}

func NewMetricsServer(opts MetricsServerOptions) (*metricsServer, error) {
	if err := opts.defaults(); err != nil {
		return nil, err
	}

	srv := &metricsServer{
		logger:  opts.Logger,
		queryer: opts.Queryer,
	}

	if opts.Addr == metricsServerDisabledAddr {
		return srv, nil
	}

	serverMux := http.NewServeMux()
	serverMux.Handle("/metrics", promhttp.Handler())
	srv.server = &http.Server{
		Addr:    opts.Addr,
		Handler: serverMux,
	}

	return srv, nil
}

func (server *metricsServer) monitorDatabaseSize(
	done <-chan struct{},
	observeFn func(sizeInBytes float64),
) {
	const dbSizeQuery = `SELECT
	page_count * page_size
	FROM pragma_page_count(), pragma_page_size();`

	observe := func() {
		var size int64
		err := server.queryer.QueryRowxContext(context.Background(), dbSizeQuery).Scan(&size)
		if err != nil {
			server.logger.Error(err, "failed to get database size")
			return
		}

		observeFn(float64(size))
	}
	observe()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			observe()
		}
	}
}

func (server *metricsServer) Start(done <-chan struct{}) {
	if server.server == nil {
		server.logger.V(8).Info("metrics server is disabled")
		return
	}

	go server.monitorDatabaseSize(done, func(sizeInBytes float64) {
		metricsDatabaseSize.Set(sizeInBytes)
		server.logger.V(8).Info("database size", "sizeInBytes", sizeInBytes)
	})
	go server.server.ListenAndServe()

	server.logger.Info("metrics server started", "addr", server.server.Addr)
	<-done

	server.logger.Info("shutting metrics server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.server.Shutdown(shutdownCtx)
}

const (
	metricsNamespace            = "sqlite_rest"
	metricsLabelTarget          = "target"    // name of the table/view
	metricsLabelTargetOperation = "operation" // name of the operation
	metricsLabelHTTPCode        = "http_code" // HTTP response code
)

var (
	metricsAuthFailedRequestsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "auth_failed_requests_total",
			Help:      "Total number of failed authentication requests",
		},
	)

	metricsAccessCheckFailedRequestsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "access_check_failed_requests_total",
			Help:      "Total number of failed access check requests",
		},
	)

	metricsRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		},
		[]string{metricsLabelTarget, metricsLabelTargetOperation, metricsLabelHTTPCode},
	)

	metricsRequestLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricsNamespace,
			Name:      "http_request_duration_milliseconds",
			Help:      "HTTP request latency",
			Buckets:   []float64{1, 10, 100, 500, 1000},
		},
		[]string{metricsLabelTarget, metricsLabelTargetOperation, metricsLabelHTTPCode},
	)

	metricsDatabaseSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricsNamespace,
			Name:      "database_size_bytes",
			Help:      "Size of the database file",
		},
	)
)

func recordRequestMetrics(op string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				httpCode := fmt.Sprint(ww.Status())
				target := chi.URLParam(r, routeVarTableOrView)
				metricsRequestTotal.
					WithLabelValues(target, op, httpCode).
					Inc()
				metricsRequestLatency.
					WithLabelValues(target, op, httpCode).
					Observe(float64(time.Since(start).Milliseconds()))
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

type PprofServerOptions struct {
	Logger logr.Logger
	Addr   string
}

func (opts *PprofServerOptions) bindCLIFlags(fs *pflag.FlagSet) {
	fs.StringVar(
		&opts.Addr, "pprof-addr", pprofServerDisabledAddr,
		"pprof server listen address. Empty value means disabled.",
	)
}

func (opts *PprofServerOptions) defaults() error {
	if opts.Logger.GetSink() == nil {
		opts.Logger = logr.Discard()
	}

	return nil
}

type pprofServer struct {
	logger logr.Logger
	server *http.Server
}

func NewPprofServer(opts PprofServerOptions) (*pprofServer, error) {
	if err := opts.defaults(); err != nil {
		return nil, err
	}

	srv := &pprofServer{
		logger: opts.Logger,
	}

	if opts.Addr == pprofServerDisabledAddr {
		return srv, nil
	}

	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/debug/pprof/", pprof.Index)
	serverMux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	serverMux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	serverMux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	serverMux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv.server = &http.Server{
		Addr:    opts.Addr,
		Handler: serverMux,
	}

	return srv, nil
}

func (server *pprofServer) Start(done <-chan struct{}) {
	if server.server == nil {
		return
	}

	server.logger.Info("pprof server is enabled, make sure it's not exposed to the public internet")

	go server.server.ListenAndServe()

	server.logger.Info("pprof server started", "addr", server.server.Addr)
	<-done

	server.logger.Info("shutting pprof server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.server.Shutdown(shutdownCtx)
}
