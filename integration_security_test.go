package main

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityNegativeCases(t *testing.T) {
	t.Run("Unauthorized", func(t *testing.T) {
		tc := createTestContextWithHMACTokenAuth(t)
		defer tc.CleanUp(t)

		tc.authToken = "" // disable auth
		client := tc.Client()
		_, _, err := client.From("test").Select("id", "", false).Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Unauthorized")
	})

	t.Run("TableAccessRestricted", func(t *testing.T) {
		tc := createTestContextWithHMACTokenAuth(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From(tableNameMigrations).Select("id", "", false).Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Access Restricted")
	})
}

func TestSecuritySQLInjection(t *testing.T) {
	t.Run("Update", func(t *testing.T) {
		tc := createTestContextWithHMACTokenAuth(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")
		tc.ExecuteSQL(t, "insert into test values (1)")

		p := bytes.NewBufferString(`{"id": 2}`)
		req := tc.NewRequest(t, http.MethodPost, "test", p)
		req.Header.Set("content-type", "application/json")
		q := req.URL.Query()
		q.Set("select", "1; drop table test;select *")
		req.URL.RawQuery = q.Encode()

		resp := tc.ExecuteRequest(t, req)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		_, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)

		client := tc.Client()
		res, _, err := client.From("test").Select("*", "", false).Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 2)
	})

	t.Run("Select", func(t *testing.T) {
		tc := createTestContextWithHMACTokenAuth(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")
		tc.ExecuteSQL(t, "insert into test values (1)")

		req := tc.NewRequest(t, http.MethodGet, "test", nil)
		req.Header.Set("content-type", "application/json")
		q := req.URL.Query()
		q.Set("select", "1; drop table test;select *")
		req.URL.RawQuery = q.Encode()

		resp := tc.ExecuteRequest(t, req)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		_, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)

		client := tc.Client()
		res, _, err := client.From("test").Select("*", "", false).Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 1)
	})
}
