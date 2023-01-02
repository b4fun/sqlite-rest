package main

import (
	"context"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/pflag"
)

const metricsServerDisabledAddr = ""

type MetricsServerOptions struct {
	Logger logr.Logger
	Addr   string
}

func (opts *MetricsServerOptions) bindCLIFlags(fs *pflag.FlagSet) {
	fs.StringVar(
		&opts.Addr, "metrics-addr", metricsServerDisabledAddr,
		"metrics server listen address. Empty value means disabled.",
	)
}

func (opts *MetricsServerOptions) defaults() error {
	if opts.Logger.GetSink() == nil {
		opts.Logger = logr.Discard()
	}

	return nil
}

type metricsServer struct {
	logger logr.Logger
	server *http.Server
}

func NewMetricsServer(opts MetricsServerOptions) (*metricsServer, error) {
	if err := opts.defaults(); err != nil {
		return nil, err
	}

	srv := &metricsServer{
		logger: opts.Logger,
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

func (server *metricsServer) Start(done <-chan struct{}) {
	if server.server == nil {
		server.logger.V(8).Info("metrics server is disabled")
		return
	}

	go server.server.ListenAndServe()

	server.logger.Info("metrics server started", "addr", server.server.Addr)
	<-done

	server.logger.Info("shutting metrics server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.server.Shutdown(shutdownCtx)
}
