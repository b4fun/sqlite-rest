package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"
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

func (opts *ServerOptions) defaults() error {
	if opts.Logger.GetSink() == nil {
		opts.Logger = logr.Discard()
	}

	if opts.Addr == "" {
		opts.Addr = "127.0.0.1:8080"
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

	serverMux := mux.NewRouter()

	{
		routePattern := fmt.Sprintf("/{%s:[^/]+}", routeVarTableOrView)
		serverMux.
			HandleFunc(routePattern, rv.handleQueryTableOrView).
			Methods("GET")
		serverMux.
			HandleFunc(routePattern, rv.handleMutateTableOrView).
			Methods("POST")
		// TODO: upsert / delete
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

type errorResponse struct {
	Message string `json:"message,omitempty"`
	Hints   string `json:"hints,omitempty"`
}

func (server *dbServer) responseError(w http.ResponseWriter, err error) {
	// TODO: reflect error for status
	resp := &errorResponse{Message: err.Error()}
	server.responseData(w, resp, 500)
}

func (server *dbServer) responseData(w http.ResponseWriter, data interface{}, statusCode int) {
	w.WriteHeader(statusCode)

	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	if encodeErr := enc.Encode(data); encodeErr != nil {
		server.logger.Error(encodeErr, "failed to write response")
		w.WriteHeader(500)
		return
	}
}

func (server *dbServer) handleQueryTableOrView(
	w http.ResponseWriter,
	req *http.Request,
) {
	vars := mux.Vars(req)
	target := vars[routeVarTableOrView]

	logger := server.logger.WithValues("target", target)

	logger.Info("handleQueryTableOrView")

	// `select [] from [] where [] order by [] limit 10 offset 20`,

	qc := &queryCompiler{req: req}
	selectStmt := qc.CompileAsSelect(target)
	logger.V(8).Info(selectStmt.Query)

	rows, err := server.queryer.QueryxContext(req.Context(), selectStmt.Query, selectStmt.Values...)
	if err != nil {
		server.responseError(w, err)
		return
	}
	defer rows.Close()

	var rv []map[string]interface{}
	for rows.Next() {
		p := make(map[string]interface{})
		if err := rows.MapScan(p); err != nil {
			server.responseError(w, err)
			return
		}
		rv = append(rv, p)
	}

	w.Header().Set("Content-Type", "application/json") // TODO: horner request config
	server.responseData(w, rv, 200)
}

func (server *dbServer) handleMutateTableOrView(
	w http.ResponseWriter,
	req *http.Request,
) {
	server.logger.Info("handleMutateTableOrView")
}

const queryParameterNameSelect = "select"

type CompiledQueryParameter struct {
	Expr   string
	Values []interface{}
}

type CompiledQuery struct {
	Query  string
	Values []interface{}
}

func (q CompiledQuery) String() string {
	return fmt.Sprintf("quey=%q values=%v", q.Query, q.Values)
}

type queryOpereatorUserInputParseFunc func(column string, userInput string, value string) []CompiledQueryParameter

func mapUserInputAsUnaryQuery(op string) queryOpereatorUserInputParseFunc {
	return func(column string, userInput string, value string) []CompiledQueryParameter {
		return []CompiledQueryParameter{
			{
				Expr:   fmt.Sprintf("%s %s ?", column, op),
				Values: []interface{}{value},
			},
		}
	}
}

var queryOpereators = map[string]queryOpereatorUserInputParseFunc{
	"eq": mapUserInputAsUnaryQuery("="),
	"gt": mapUserInputAsUnaryQuery(">"), "ge": mapUserInputAsUnaryQuery(">="),
	"lt": mapUserInputAsUnaryQuery("<"), "le": mapUserInputAsUnaryQuery("<="),
	"neq":  mapUserInputAsUnaryQuery("!="),
	"like": mapUserInputAsUnaryQuery("LIKE"), "ilike": mapUserInputAsUnaryQuery("ILIKE"),
	// "in": "in", // TODO: support query operator parser
}

type queryCompiler struct {
	req *http.Request
}

func (c *queryCompiler) getQueryParameters(name string) []string {
	qp := c.req.URL.Query()
	if !qp.Has(name) {
		return nil
	}
	return qp[name]
}

func (c *queryCompiler) getQueryParameter(name string) string {
	qp := c.req.URL.Query()
	if !qp.Has(name) {
		return ""
	}
	return qp.Get(name)
}

func (c *queryCompiler) CompileAsSelect(table string) CompiledQuery {
	rv := CompiledQuery{}

	rv.Query = fmt.Sprintf(
		"select %s from %s",
		strings.Join(c.GetSelectResultColumns(), ", "),
		table,
	)

	var qcs []string
	for _, qc := range c.GetQueryClauses() {
		qcs = append(qcs, qc.Expr)
		rv.Values = append(rv.Values, qc.Values...)
	}
	if len(qcs) > 0 {
		rv.Query = fmt.Sprintf("%s where %s", rv.Query, strings.Join(qcs, " and "))
	}

	return rv
}

func (c *queryCompiler) GetSelectResultColumns() []string {
	v := c.getQueryParameter(queryParameterNameSelect)
	if v == "" {
		return []string{"*"}
	}

	vs := strings.Split(v, ",")
	// TOOD: support renaming, casting

	return vs
}

func (c *queryCompiler) GetQueryClauses() []CompiledQueryParameter {
	var rv []CompiledQueryParameter
	for k := range c.req.URL.Query() {
		if !c.isColumnName(k) {
			continue
		}

		vs := c.getQueryClausesByColumn(k)
		if len(vs) < 1 {
			continue
		}

		rv = append(rv, vs...)
	}

	return rv
}

func (c *queryCompiler) isColumnName(s string) bool {
	switch strings.ToLower(s) {
	case queryParameterNameSelect:
		return false
	default:
		return true
	}
}

func (c *queryCompiler) getQueryClausesByColumn(column string) []CompiledQueryParameter {
	vs := c.getQueryParameters(column)
	if len(vs) < 1 {
		return nil
	}

	var rv []CompiledQueryParameter
	for _, v := range vs {
		ps := c.getQueryClausesByInput(column, v)
		if len(ps) < 1 {
			continue
		}
		rv = append(rv, ps...)
	}

	return rv
}

func (c *queryCompiler) getQueryClausesByInput(column string, s string) []CompiledQueryParameter {
	if s == "" {
		return nil
	}

	// eq.1 => `eq 1`
	ps := strings.SplitN(s, ".", 2)
	op, userInput := ps[0], ps[1]
	if op == "" || userInput == "" {
		return nil
	}

	if p, exists := queryOpereators[op]; exists {
		return p(column, op, userInput)
	}

	return nil
}

func createServeCmd() *cobra.Command {
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

			opts := &ServerOptions{
				Logger:  logger,
				Queryer: db,
				Execer:  db,
			}

			server, err := NewServer(opts)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			server.Start(ctx.Done())

			return nil
		},
	}

	return cmd
}
