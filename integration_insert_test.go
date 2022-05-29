package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testInsert_SingleTable(t *testing.T, createTestContext func(t testing.TB) *TestContext) {
	t.Run("NoTable", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From("test").
			Insert(map[string]interface{}{"id": 1}, false, "", "", "").
			Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table: test")
	})

	t.Run("InsertSingleValue", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")

		client := tc.Client()

		_, _, err := client.From("test").
			Insert(map[string]interface{}{"id": 1}, false, "", "", "").
			Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 1)
		assert.EqualValues(t, 1, rv[0]["id"])
	})

	t.Run("InsertValues", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")

		client := tc.Client()

		_, _, err := client.From("test").
			Insert([]map[string]interface{}{{"id": 1}, {"id": 1}}, false, "", "", "").
			Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 2)
		for _, row := range rv {
			assert.EqualValues(t, 1, row["id"])
		}
	})

	t.Run("UpsertMergeDuplicates", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int primary key, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) values (1, "a"), (2, "b")`)

		client := tc.Client()

		_, _, err := client.From("test").
			Insert([]map[string]interface{}{
				{"id": 1, "s": "b"}, {"id": 2, "s": "c"},
			}, true, "", "", "").
			Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("*", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 2)
		for idx, row := range rv {
			assert.EqualValues(t, idx+1, row["id"])
			assert.EqualValues(t, string('a'+rune(idx+1)), row["s"])
		}
	})

	t.Run("UpsertIgnoreDuplicates", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int primary key, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) values (1, "a"), (2, "b")`)

		payload := bytes.NewBufferString(`[{"id": 1, "s": "b"}, {"id": 2, "s": "c"}]`)
		req := tc.NewRequest(t, http.MethodPost, "test", payload)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Prefer", "resolution=ignore-duplicates")
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
			assert.EqualValues(t, string('a'+rune(idx)), row["s"])
		}
	})

	t.Run("UpsertMergeDuplicatesWithOnConflicts", func(t *testing.T) {
		tc := createTestContext(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, "CREATE UNIQUE INDEX test_id on test (id)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) values (1, "a"), (2, "b")`)

		client := tc.Client()

		_, _, err := client.From("test").
			Insert([]map[string]interface{}{
				{"id": 1, "s": "b"}, {"id": 2, "s": "c"},
			}, true, "id", "", "").
			Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("*", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 2)
		for idx, row := range rv {
			assert.EqualValues(t, idx+1, row["id"])
			assert.EqualValues(t, string('a'+rune(idx+1)), row["s"])
		}
	})
}

func TestInsert_SingleTable(t *testing.T) {
	t.Run("in memory db", func(t *testing.T) {
		testInsert_SingleTable(t, createTestContextUsingInMemoryDB)
	})

	t.Run("HMAC token auth", func(t *testing.T) {
		testInsert_SingleTable(t, createTestContextWithHMACTokenAuth)
	})

	t.Run("RSA token auth", func(t *testing.T) {
		testDelete_SingleTable(t, createTestContextWithRSATokenAuth)
	})
}
