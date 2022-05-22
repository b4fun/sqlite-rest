package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
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
			Methods(http.MethodGet)
		serverMux.
			HandleFunc(routePattern, rv.handleInsertTable).
			Methods(http.MethodPost)
		serverMux.
			HandleFunc(routePattern, rv.handleUpdateTable).
			Methods(http.MethodPatch)
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
	vars := mux.Vars(req)
	target := vars[routeVarTableOrView]

	logger := server.logger.WithValues("target", target, "route", "handleQueryTableOrView")

	qc := &queryCompiler{req: req}
	selectStmt, err := qc.CompileAsSelect(target)
	if err != nil {
		logger.Error(err, "parse select query")
		server.responseError(w, err)
		return
	}
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
	server.responseData(w, rv, http.StatusOK)
}

func (server *dbServer) handleInsertTable(
	w http.ResponseWriter,
	req *http.Request,
) {
	vars := mux.Vars(req)
	target := vars[routeVarTableOrView]

	logger := server.logger.WithValues("target", target, "route", "handleInsertTable")

	qc := &queryCompiler{req: req}
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
	vars := mux.Vars(req)
	target := vars[routeVarTableOrView]

	logger := server.logger.WithValues("target", target, "route", "handleUpdateTable")

	qc := &queryCompiler{req: req}
	updateStmt, err := qc.CompileAsUpdate(target)
	if err != nil {
		logger.Error(err, "parse insert query")
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

type InputPayloadWithColumns struct {
	Columns map[string]struct{}
	Payload []map[string]interface{}
}

func (p InputPayloadWithColumns) GetSortedColumns() []string {
	columns := make([]string, 0, len(p.Columns))
	for column := range p.Columns {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

func (p InputPayloadWithColumns) GetValues(columns []string) [][]interface{} {
	var rv [][]interface{}
	for _, p := range p.Payload {
		var row []interface{}
		for _, column := range columns {
			v, exists := p[column]
			if exists {
				row = append(row, v)
			} else {
				row = append(row, nil)
			}
		}
		rv = append(rv, row)
	}

	return rv
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

func (c *queryCompiler) CompileAsSelect(table string) (CompiledQuery, error) {
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

	return rv, nil
}

func (c *queryCompiler) CompileAsUpdate(table string) (CompiledQuery, error) {
	rv := CompiledQuery{}

	payload, err := c.GetInputPayload()
	if err != nil {
		return rv, err
	}
	if len(payload.Columns) < 1 {
		return rv, ErrBadRequest.WithHints("no columns to insert")
	}
	if len(payload.Payload) < 1 {
		return rv, ErrBadRequest.WithHints("no data to insert")
	}
	if len(payload.Payload) > 1 {
		return rv, ErrBadRequest.WithHints("too many data to update")
	}

	columns := payload.GetSortedColumns()
	updateValues := payload.Payload[0]
	var columnPlaceholders []string
	for _, column := range columns {
		columnPlaceholders = append(columnPlaceholders, fmt.Sprintf("%s = ?", column))
		rv.Values = append(rv.Values, updateValues[column])
	}

	rv.Query = fmt.Sprintf(
		"update %s set %s",
		table,
		strings.Join(columnPlaceholders, ", "),
	)

	var qcs []string
	for _, qc := range c.GetQueryClauses() {
		qcs = append(qcs, qc.Expr)
		rv.Values = append(rv.Values, qc.Values...)
	}
	if len(qcs) > 0 {
		rv.Query = fmt.Sprintf("%s where %s", rv.Query, strings.Join(qcs, " and "))
	}

	return rv, nil
}

func (c *queryCompiler) CompileAsInsert(table string) (CompiledQuery, error) {
	rv := CompiledQuery{}

	payload, err := c.GetInputPayload()
	if err != nil {
		return rv, err
	}
	if len(payload.Columns) < 1 {
		return rv, ErrBadRequest.WithHints("no columns to insert")
	}
	if len(payload.Payload) < 1 {
		return rv, ErrBadRequest.WithHints("no data to insert")
	}

	columns := payload.GetSortedColumns()

	values := payload.GetValues(columns)
	var valuePlaceholders []string
	for range values {
		valuePlaceholders = append(
			valuePlaceholders,
			fmt.Sprintf("(%s?)", strings.Repeat("?, ", len(columns)-1)),
		)
	}

	rv.Query = fmt.Sprintf(
		`insert into %s (%s) values %s`,
		table,
		strings.Join(columns, ", "),
		strings.Join(valuePlaceholders, ", "),
	)

	for _, v := range values {
		rv.Values = append(rv.Values, v...)
	}

	return rv, nil
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

func (c *queryCompiler) GetInputPayload() (InputPayloadWithColumns, error) {
	contentType := c.req.Header.Get("content-type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	for _, v := range strings.Split(contentType, ",") {
		mt, _, err := mime.ParseMediaType(v)
		if err != nil {
			continue
		}

		switch strings.ToLower(mt) {
		case "application/json":
			payload, err := c.tryReadInputPayloadAsJSON()
			if err != nil {
				continue
			}
			return payload, nil
		default:
			continue
		}
	}

	return InputPayloadWithColumns{}, ErrUnsupportedMediaType
}

func (c *queryCompiler) tryReadInputPayloadAsJSON() (InputPayloadWithColumns, error) {
	rv := InputPayloadWithColumns{
		Columns: map[string]struct{}{},
	}

	body, err := c.readyRequestBody()
	if err != nil {
		return rv, err
	}

	// TODO: we need a Peek method from json.Decoder
	enc := json.NewDecoder(bytes.NewBuffer(body))
	tok, err := enc.Token()
	if err != nil {
		return rv, err
	}
	switch tok {
	case json.Delim('['):
		// a json array
		var ps []map[string]interface{}
		if err := json.Unmarshal(body, &ps); err != nil {
			return rv, err
		}
		rv.Payload = append(rv.Payload, ps...)
	default:
		// try as single object
		var p map[string]interface{}
		if err := json.Unmarshal(body, &p); err != nil {
			return rv, err
		}
		rv.Payload = append(rv.Payload, p)
	}

	for _, p := range rv.Payload {
		for k := range p {
			rv.Columns[k] = struct{}{}
		}
	}

	return rv, nil
}

func (c *queryCompiler) readyRequestBody() ([]byte, error) {
	source := c.req.Body
	defer source.Close()
	b, err := io.ReadAll(source)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	c.req.Body = io.NopCloser(bytes.NewBuffer(b))

	return b, nil
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
			defer db.Close()

			opts := &ServerOptions{
				Logger:  logger,
				Queryer: db,
				Execer:  db,
			}

			server, err := NewServer(opts)
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

	return cmd
}
