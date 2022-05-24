package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
)

type CompiledQuery struct {
	Query  string
	Values []interface{}
}

func (q CompiledQuery) String() string {
	return fmt.Sprintf("quey=%q values=%v", q.Query, q.Values)
}

type QueryCompiler interface {
	CompileAsSelect(table string) (CompiledQuery, error)
	CompileAsUpdate(table string) (CompiledQuery, error)
	CompileAsInsert(table string) (CompiledQuery, error)
	CompileAsDelete(table string) (CompiledQuery, error)
}

type queryCompiler struct {
	req *http.Request
}

func NewQueryCompilerFromRequest(req *http.Request) QueryCompiler {
	return &queryCompiler{req: req}
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
		strings.Join(c.getSelectResultColumns(), ", "),
		table,
	)

	var qcs []string
	for _, qc := range c.getQueryClauses() {
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

	payload, err := c.getInputPayload()
	if err != nil {
		return rv, err
	}
	if len(payload.Columns) < 1 {
		return rv, ErrBadRequest.WithHint("no columns to insert")
	}
	if len(payload.Payload) < 1 {
		return rv, ErrBadRequest.WithHint("no data to insert")
	}
	if len(payload.Payload) > 1 {
		return rv, ErrBadRequest.WithHint("too many data to update")
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
	for _, qc := range c.getQueryClauses() {
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

	payload, err := c.getInputPayload()
	if err != nil {
		return rv, err
	}
	if len(payload.Columns) < 1 {
		return rv, ErrBadRequest.WithHint("no columns to insert")
	}
	if len(payload.Payload) < 1 {
		return rv, ErrBadRequest.WithHint("no data to insert")
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

func (c *queryCompiler) CompileAsDelete(table string) (CompiledQuery, error) {
	rv := CompiledQuery{}

	rv.Query = fmt.Sprintf(`delete from %s`, table)

	var qcs []string
	for _, qc := range c.getQueryClauses() {
		qcs = append(qcs, qc.Expr)
		rv.Values = append(rv.Values, qc.Values...)
	}
	if len(qcs) > 0 {
		rv.Query = fmt.Sprintf("%s where %s", rv.Query, strings.Join(qcs, " and "))
	}

	return rv, nil
}

func (c *queryCompiler) getSelectResultColumns() []string {
	v := c.getQueryParameter(queryParameterNameSelect)
	if v == "" {
		return []string{"*"}
	}

	vs := strings.Split(v, ",")
	// TOOD: support renaming, casting

	return vs
}

func (c *queryCompiler) getQueryClauses() []CompiledQueryParameter {
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

func (c *queryCompiler) getInputPayload() (InputPayloadWithColumns, error) {
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

const queryParameterNameSelect = "select"

type CompiledQueryParameter struct {
	Expr   string
	Values []interface{}
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
