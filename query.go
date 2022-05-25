package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const (
	queryParameterNameSelect = "select"
	queryParameterNameOrder  = "order"
	queryParameterNameLimit  = "limit"
	queryParameterNameOffset = "offset"

	headerNameRangeUnit = "range-unit"
	headerNameRange     = "range"
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
	CompileContentRangeHeader(totalCount string) string
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

	var queryClauses []string
	for _, qc := range c.getQueryClauses() {
		queryClauses = append(queryClauses, qc.Expr)
		rv.Values = append(rv.Values, qc.Values...)
	}
	if len(queryClauses) > 0 {
		rv.Query = fmt.Sprintf("%s where %s", rv.Query, strings.Join(queryClauses, " and "))
	}

	orderClauses, err := c.getOrderClauses()
	if err != nil {
		return rv, err
	}
	if len(orderClauses) > 0 {
		rv.Query = fmt.Sprintf("%s order by %s", rv.Query, strings.Join(orderClauses, ", "))
	}

	limit, offset, err := c.getLimitOffset()
	switch {
	case err == nil:
		rv.Query = fmt.Sprintf("%s limit %d", rv.Query, limit)
		if offset != 0 {
			rv.Query = fmt.Sprintf("%s offset %d", rv.Query, offset)
		}
	case errors.Is(err, errNoLimitOffset):
		// no limit/offset
	default:
		return rv, err
	}

	fmt.Println(rv)
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
	case queryParameterNameSelect,
		queryParameterNameOffset,
		queryParameterNameLimit,
		queryParameterNameOrder:
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

var orderByNulls = map[string]string{
	"nullslast":  "nulls last",
	"nullsfirst": "nulls first",
}

func (c *queryCompiler) getOrderClauses() ([]string, error) {
	v := c.getQueryParameter(queryParameterNameOrder)
	if v == "" {
		return nil, nil
	}

	translateOrderBy := func(s string) string {
		if v, exists := orderByNulls[s]; exists {
			return v
		}
		return s
	}

	var vs []string
	for _, v := range strings.Split(v, ",") {
		ps := strings.Split(v, ".")
		switch {
		case len(ps) == 1:
			vs = append(vs, ps[0])
		case len(ps) == 2:
			// a.asc -> a asc
			// a.nullslast -> a nulls last
			vs = append(vs, fmt.Sprintf("%s %s", ps[0], translateOrderBy(ps[1])))
		case len(ps) == 3:
			// a.asc.nullslast
			vs = append(vs, fmt.Sprintf("%s %s %s", ps[0], ps[1], translateOrderBy(ps[2])))
		default:
			// invalid
			return nil, fmt.Errorf("invalid order by clause: %s", v)
		}
	}

	return vs, nil
}

var errNoLimitOffset = errors.New("no limit offset")

func (c *queryCompiler) CompileContentRangeHeader(totalCount string) string {
	limit, offset, err := c.getLimitOffset()
	if err != nil {
		// unable to infer limit/offset
		return ""
	}

	if limit < 0 {
		// unbound range
		return fmt.Sprintf("%d-/%s", offset, totalCount)
	}

	return fmt.Sprintf("%d-%d/%s", offset, offset+limit-1, totalCount)
}

func (c *queryCompiler) getLimitOffset() (limit int64, offset int64, err error) {
	limit, offset, err = c.getLimitOffsetFromHeader()
	if err == nil {
		return limit, offset, nil
	}
	if !errors.Is(err, errNoLimitOffset) {
		return 0, 0, err
	}
	return c.getLimitOffsetFromQueryParameter()
}

func (c *queryCompiler) getLimitOffsetFromHeader() (int64, int64, error) {
	rangeValue := c.req.Header.Get(headerNameRange)
	if rangeValue == "" {
		return 0, 0, errNoLimitOffset
	}

	ps := strings.SplitN(rangeValue, "-", 2)
	if len(ps) < 1 {
		return 0, 0, errNoLimitOffset
	}

	offset, err := strconv.ParseInt(ps[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	if ps[1] == "" {
		// no limit, per: https://www.sqlite.org/lang_select.html#limitoffset
		// If the LIMIT expression evaluates to a negative value,
		// then there is no upper bound on the number of rows returned
		return -1, offset, nil
	}
	to, err := strconv.ParseInt(ps[1], 10, 64)
	if err != nil {
		return 0, 0, err
	}

	return to - offset + 1, offset, nil
}

func (c *queryCompiler) getLimitOffsetFromQueryParameter() (int64, int64, error) {
	getInt64 := func(qp string) (int64, error) {
		v := c.getQueryParameter(qp)
		if v == "" {
			return 0, errNoLimitOffset
		}
		return strconv.ParseInt(v, 10, 64)
	}

	limit, err := getInt64(queryParameterNameLimit)
	if err != nil {
		return 0, 0, err
	}
	offset, err := getInt64(queryParameterNameOffset)
	switch {
	case err == nil:
		return limit, offset, nil
	case errors.Is(err, errNoLimitOffset):
		// offset is optional
		return limit, 0, nil
	default:
		return 0, 0, err
	}
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