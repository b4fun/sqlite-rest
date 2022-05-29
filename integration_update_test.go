package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testUpdate_SingleTable(t *testing.T, createTestContext func(t testing.TB) *TestContext) {
	t.Run("NoTable", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From("test").Update(map[string]interface{}{"id": 1}, "", "1").
			Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table: test")
	})

	t.Run("UpdateRecords", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (1, "a"), (1, "a")`)

		client := tc.Client()
		_, _, err := client.From("test").Update(map[string]interface{}{"id": 2}, "", "3").
			Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 3)
		for _, row := range rv {
			assert.EqualValues(t, 2, row["id"])
		}
	})

	t.Run("UpdateWithFilter", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (1, "a"), (1, "a")`)

		client := tc.Client()
		_, _, err := client.From("test").
			Update(map[string]interface{}{"id": 2}, "", "3").
			Eq("id", "100").
			Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 3)
		for _, row := range rv {
			assert.EqualValues(t, 1, row["id"])
		}
	})

	t.Run("UpdateSingleEntry", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (2, "c")`)

		b := bytes.NewBufferString(`{"id": 1, "s": "b"}`)
		req := tc.NewRequest(t, http.MethodPut, "test", b)
		req.Header.Set("Content-Type", "application/json")
		q := req.URL.Query()
		q.Set("id", "eq.1")
		req.URL.RawQuery = q.Encode()

		resp := tc.ExecuteRequest(t, req)
		defer resp.Body.Close()

		client := tc.Client()
		res, _, err := client.From("test").Select("*", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 2)
		for idx, row := range rv {
			assert.EqualValues(t, idx+1, row["id"])
			assert.EqualValues(t, string('b'+rune(idx)), row["s"])
		}
	})
}

func TestUpdate_SingleTable(t *testing.T) {
	t.Run("in memory db", func(t *testing.T) {
		testUpdate_SingleTable(t, createTestContextUsingInMemoryDB)
	})

	t.Run("HMAC token auth", func(t *testing.T) {
		testUpdate_SingleTable(t, createTestContextWithHMACTokenAuth)
	})

	t.Run("RSA token auth", func(t *testing.T) {
		testUpdate_SingleTable(t, createTestContextWithRSATokenAuth)
	})
}
