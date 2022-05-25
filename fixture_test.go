package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/supabase/postgrest-go"
	"k8s.io/klog/v2/ktesting"
)

type TestContext struct {
	server    *httptest.Server
	db        *sqlx.DB
	cleanUpDB func(t testing.TB)
}

func NewTestContextWithDB(
	t testing.TB,
	handler http.Handler,
	db *sqlx.DB,
	cleanUpDB func(t testing.TB),
) *TestContext {
	rv := &TestContext{
		server:    httptest.NewServer(handler),
		db:        db,
		cleanUpDB: cleanUpDB,
	}

	return rv
}

func (tc *TestContext) CleanUp(t testing.TB) {
	if tc.cleanUpDB != nil {
		tc.cleanUpDB(t)
	}

	tc.server.Close()
}

func (tc *TestContext) DB() *sqlx.DB {
	return tc.db
}

func (tc *TestContext) ServerURL() *url.URL {
	u, err := url.Parse(tc.server.URL)
	if err != nil {
		// shouldn't happen
		panic(fmt.Sprintf("failed to parse server url: %s", err))
	}
	return u
}

func (tc *TestContext) Client() *postgrest.Client {
	return postgrest.NewClient(
		tc.ServerURL().String(),
		"http",
		nil,
	)
}

func (tc *TestContext) HTTPClient() *http.Client {
	return &http.Client{}
}

func (tc *TestContext) NewRequest(
	t testing.TB,
	method string, path string,
	body io.Reader,
) *http.Request {
	req, err := http.NewRequest(method, tc.ServerURL().String()+"/"+path, body)
	assert.NoError(t, err)
	return req
}

func (tc *TestContext) ExecuteRequest(t testing.TB, req *http.Request) *http.Response {
	resp, err := tc.HTTPClient().Do(req)
	assert.NoError(t, err)
	return resp
}

func (tc *TestContext) ExecuteSQL(t testing.TB, stmt string, args ...interface{}) {
	_, err := tc.DB().Exec(stmt, args...)
	assert.NoError(t, err)
}

func (tc *TestContext) DecodeResult(t testing.TB, res []byte, des interface{}) {
	err := json.Unmarshal(res, des)
	assert.NoError(t, err)
}

func createTestLogger(t testing.TB) logr.Logger {
	return ktesting.NewLogger(t, ktesting.NewConfig(ktesting.Verbosity(12)))
}

func createTestContextUsingInMemoryDB(t testing.TB) *TestContext {
	t.Log("creating in-memory db")
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Error(err)
		return nil
	}

	t.Log("creating server")
	serverOpts := &ServerOptions{
		Logger:  createTestLogger(t).WithName("test"),
		Queryer: db,
		Execer:  db,
	}
	server, err := NewServer(serverOpts)
	if err != nil {
		t.Error(err)
		return nil
	}

	return NewTestContextWithDB(
		t,
		server.server.Handler,
		db,
		func(t testing.TB) {
			if err := db.Close(); err != nil {
				t.Errorf("closing in-memory db: %s", err)
			}
		})
}