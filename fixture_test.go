package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang-jwt/jwt"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/supabase/postgrest-go"
	"k8s.io/klog/v2/ktesting"
)

var enabledTestTables = []string{"test", "test_view"}

type TestContext struct {
	server    *httptest.Server
	db        *sqlx.DB
	cleanUpDB func(t testing.TB)
	authToken string
}

func NewTestContextWithDB(
	t testing.TB,
	handler http.Handler,
	db *sqlx.DB,
	cleanUpDB func(t testing.TB),
	authToken string,
) *TestContext {
	rv := &TestContext{
		server:    httptest.NewServer(handler),
		db:        db,
		cleanUpDB: cleanUpDB,
		authToken: authToken,
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
	rv := postgrest.NewClient(
		tc.ServerURL().String(),
		"http",
		nil,
	)

	if tc.authToken != "" {
		rv = rv.TokenAuth(tc.authToken)
	}

	return rv
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

	if tc.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+tc.authToken)
	}
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
		t.Fatal(err)
		return nil
	}

	t.Log("creating server")
	serverOpts := &ServerOptions{
		Logger:  createTestLogger(t).WithName("test"),
		Queryer: db,
		Execer:  db,
	}
	serverOpts.AuthOptions.disableAuth = true
	serverOpts.SecurityOptions.EnabledTableOrViews = enabledTestTables
	server, err := NewServer(serverOpts)
	if err != nil {
		t.Fatal(err)
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
		},
		"",
	)
}

func createTestContextWithHMACTokenAuth(t testing.TB) *TestContext {
	t.Log("creating test dir")
	dir, err := os.MkdirTemp("", "sqlite-rest-test")
	if err != nil {
		t.Fatal(err)
		return nil
	}

	t.Log("creating test token file")
	testToken := []byte("test-token")
	testTokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(testTokenFile, testToken, 0644); err != nil {
		t.Fatal(err)
		return nil
	}

	authToken := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.StandardClaims{})
	authTokenString, err := authToken.SignedString(testToken)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	db, err := sqlx.Open("sqlite3", "//"+filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
		return nil
	}

	t.Log("creating server")
	serverOpts := &ServerOptions{
		Logger:  createTestLogger(t).WithName("test"),
		Queryer: db,
		Execer:  db,
	}
	serverOpts.AuthOptions.TokenFilePath = testTokenFile
	serverOpts.SecurityOptions.EnabledTableOrViews = enabledTestTables
	server, err := NewServer(serverOpts)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	return NewTestContextWithDB(
		t,
		server.server.Handler,
		db,
		func(t testing.TB) {
			if err := db.Close(); err != nil {
				t.Fatalf("closing db: %s", err)
				return
			}

			if err := os.RemoveAll(dir); err != nil {
				t.Fatalf("removing test dir %q: %s", dir, err)
				return
			}
		},
		authTokenString,
	)
}

func createTestContextWithRSATokenAuth(t testing.TB) *TestContext {
	t.Log("creating test dir")
	dir, err := os.MkdirTemp("", "sqlite-rest-test")
	if err != nil {
		t.Fatal(err)
		return nil
	}

	t.Log("creating test token file")
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	b, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	publicKeyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: b,
	})

	testTokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(testTokenFile, publicKeyPem, 0644); err != nil {
		t.Fatal(err)
		return nil
	}

	authToken := jwt.NewWithClaims(jwt.SigningMethodRS256, &jwt.StandardClaims{})
	authTokenString, err := authToken.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	db, err := sqlx.Open("sqlite3", "//"+filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
		return nil
	}

	t.Log("creating server")
	serverOpts := &ServerOptions{
		Logger:  createTestLogger(t).WithName("test"),
		Queryer: db,
		Execer:  db,
	}
	serverOpts.AuthOptions.RSAPublicKeyFilePath = testTokenFile
	serverOpts.SecurityOptions.EnabledTableOrViews = enabledTestTables
	server, err := NewServer(serverOpts)
	if err != nil {
		t.Fatal(err)
		return nil
	}

	return NewTestContextWithDB(
		t,
		server.server.Handler,
		db,
		func(t testing.TB) {
			if err := db.Close(); err != nil {
				t.Fatalf("closing db: %s", err)
				return
			}

			if err := os.RemoveAll(dir); err != nil {
				t.Fatalf("removing test dir %q: %s", dir, err)
				return
			}
		},
		authTokenString,
	)
}
