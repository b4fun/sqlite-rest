package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	routeVarTableOrView = "tableOrView"
)

type ServerOptions struct {
	Logger  logr.Logger
	Addr    string
	Queryer sqlx.QueryerContext
	Execer  sqlx.ExecerContext
}

func (opts *ServerOptions) bindCLIFlags(fs *pflag.FlagSet) {
	fs.StringVar(&opts.Addr, "http-addr", ":8080", "server listen addr")
}

func (opts *ServerOptions) defaults() error {
	if opts.Logger.GetSink() == nil {
		opts.Logger = logr.Discard()
	}

	if opts.Addr == "" {
		opts.Addr = ":8080"
	}

	if opts.Queryer == nil {
		return fmt.Errorf(".Queryer is required")
	}

	if opts.Execer == nil {
		return fmt.Errorf(".Execer is required")
	}

	return nil
}

type dbServer struct {
	logger  logr.Logger
	server  *http.Server
	queryer sqlx.QueryerContext
	execer  sqlx.ExecerContext
}

func NewServer(opts *ServerOptions) (*dbServer, error) {
	if err := opts.defaults(); err != nil {
		return nil, err
	}

	rv := &dbServer{
		logger: opts.Logger.WithName("db-server"),
		server: &http.Server{
			Addr: opts.Addr,
		},
		queryer: opts.Queryer,
		execer:  opts.Execer,
	}

	serverMux := chi.NewRouter()

	// TODO: allow specifying cors config from cli / table
	serverMux.Use(cors.AllowAll().Handler)

	{
		routePattern := fmt.Sprintf("/{%s:[^/]+}", routeVarTableOrView)
		serverMux.Get(routePattern, rv.handleQueryTableOrView)
		serverMux.Post(routePattern, rv.handleInsertTable)
		serverMux.Patch(routePattern, rv.handleUpdateTable)
		serverMux.Delete(routePattern, rv.handleDeleteTable)
		// TODO: upsert
	}

	rv.server.Handler = serverMux

	return rv, nil
}

func (server *dbServer) Start(done <-chan struct{}) {
	go server.server.ListenAndServe()

	server.logger.Info("server started", "addr", server.server.Addr)
	<-done

	server.logger.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	server.server.Shutdown(shutdownCtx)
}

func (server *dbServer) responseError(w http.ResponseWriter, err error) {
	var serverError *ServerError
	switch {
	case errors.As(err, &serverError):
		server.responseData(w, serverError, serverError.StatusCode)
	default:
		resp := &ServerError{Message: err.Error()}
		server.responseData(w, resp, http.StatusInternalServerError)
	}
}

func (server *dbServer) responseData(w http.ResponseWriter, data interface{}, statusCode int) {
	w.WriteHeader(statusCode)

	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	if encodeErr := enc.Encode(data); encodeErr != nil {
		server.logger.Error(encodeErr, "failed to write response")
		w.WriteHeader(http.StatusCreated)
		return
	}
}

func (server *dbServer) responseEmptyBody(w http.ResponseWriter, statusCode int) {
	w.WriteHeader(statusCode)
}

func (server *dbServer) handleQueryTableOrView(
	w http.ResponseWriter,
	req *http.Request,
) {
	target := chi.URLParam(req, routeVarTableOrView)

	logger := server.logger.WithValues("target", target, "route", "handleQueryTableOrView")

	qc := NewQueryCompilerFromRequest(req)
	selectStmt, err := qc.CompileAsSelect(target)
	if err != nil {
		logger.Error(err, "parse select query")
		server.responseError(w, err)
		return
	}
	// TODO: handle count query - use different query when requesting count
	countTotal := "*"
	logger.V(8).Info(selectStmt.Query)

	rows, err := server.queryer.QueryxContext(req.Context(), selectStmt.Query, selectStmt.Values...)
	if err != nil {
		server.responseError(w, err)
		return
	}
	defer rows.Close()

	// make sure return list instead of null for empty list
	// FIXME: reflect column type and scan typed value instead of using `interface{}`
	rv := make([]map[string]interface{}, 0)
	rows.ColumnTypes()
	for rows.Next() {
		p := make(map[string]interface{})
		if err := rows.MapScan(p); err != nil {
			server.responseError(w, err)
			return
		}
		rv = append(rv, p)
	}

	w.Header().Set("Content-Type", "application/json") // TODO: horner request config

	if v := qc.CompileContentRangeHeader(countTotal); v != "" {
		w.Header().Set("Range-Unit", "items")
		w.Header().Set("Content-Range", v)
	}

	server.responseData(w, rv, http.StatusOK)
}

func (server *dbServer) handleInsertTable(
	w http.ResponseWriter,
	req *http.Request,
) {
	target := chi.URLParam(req, routeVarTableOrView)

	logger := server.logger.WithValues("target", target, "route", "handleInsertTable")

	qc := NewQueryCompilerFromRequest(req)
	insertStmt, err := qc.CompileAsInsert(target)
	if err != nil {
		logger.Error(err, "parse insert query")
		server.responseError(w, err)
		return
	}
	logger.V(8).Info(insertStmt.Query)

	_, err = server.execer.ExecContext(req.Context(), insertStmt.Query, insertStmt.Values...)
	if err != nil {
		server.responseError(w, err)
		return
	}

	// TODO: implement support for retrieving object by inserted id
	server.responseEmptyBody(w, http.StatusCreated)
}

func (server *dbServer) handleUpdateTable(
	w http.ResponseWriter,
	req *http.Request,
) {
	target := chi.URLParam(req, routeVarTableOrView)

	logger := server.logger.WithValues("target", target, "route", "handleUpdateTable")

	qc := NewQueryCompilerFromRequest(req)
	updateStmt, err := qc.CompileAsUpdate(target)
	if err != nil {
		logger.Error(err, "parse update query")
		server.responseError(w, err)
		return
	}
	logger.V(8).Info(updateStmt.Query)

	_, err = server.execer.ExecContext(req.Context(), updateStmt.Query, updateStmt.Values...)
	if err != nil {
		server.responseError(w, err)
		return
	}

	// TODO: implement support for retrieving object by inserted id
	server.responseEmptyBody(w, http.StatusAccepted)
}

func (server *dbServer) handleDeleteTable(
	w http.ResponseWriter,
	req *http.Request,
) {
	target := chi.URLParam(req, routeVarTableOrView)

	logger := server.logger.WithValues("target", target, "route", "handleDeleteTable")

	qc := NewQueryCompilerFromRequest(req)
	updateStmt, err := qc.CompileAsDelete(target)
	if err != nil {
		logger.Error(err, "parse delete query")
		server.responseError(w, err)
		return
	}
	logger.V(8).Info(updateStmt.Query)

	_, err = server.execer.ExecContext(req.Context(), updateStmt.Query, updateStmt.Values...)
	if err != nil {
		server.responseError(w, err)
		return
	}

	server.responseEmptyBody(w, http.StatusAccepted)
}

func createServeCmd() *cobra.Command {
	serverOpts := new(ServerOptions)

	cmd := &cobra.Command{
		Use:           "serve",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, err := createLogger(cmd)
			if err != nil {
				setupLogger.Error(err, "failed to create logger")
				return err
			}

			db, err := openDB(cmd)
			if err != nil {
				setupLogger.Error(err, "failed to open db")
				return err
			}
			defer db.Close()

			serverOpts.Logger = logger
			serverOpts.Queryer = db
			serverOpts.Execer = db

			server, err := NewServer(serverOpts)
			if err != nil {
				setupLogger.Error(err, "failed to create server")
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			server.Start(ctx.Done())

			return nil
		},
	}
	serverOpts.bindCLIFlags(cmd.Flags())

	return cmd
}
