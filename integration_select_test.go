package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase/postgrest-go"
)

func TestSelect_SingleTable(t *testing.T) {
	t.Run("NoTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table: test")
	})

	t.Run("EmptyTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")

		client := tc.Client()
		res, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Empty(t, rv)
	})

	t.Run("SelectAllColumns", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (1, "a"), (1, "a")`)

		client := tc.Client()
		res, _, err := client.From("test").Select("*", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 3)
		for _, row := range rv {
			assert.EqualValues(t, 1, row["id"])
			assert.EqualValues(t, "a", row["s"])
		}
	})

	t.Run("SelectSingleColumn", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (1, "a"), (1, "a")`)

		client := tc.Client()
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

	t.Run("SelectWithFilter", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")
		tc.ExecuteSQL(t, `INSERT INTO test (id) VALUES (1), (2), (3)`)

		client := tc.Client()
		res, _, err := client.From("test").Select("id", "", false).
			Eq("id", "1").
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 1)
		assert.EqualValues(t, 1, rv[0]["id"])
	})

	t.Run("SelectWithOrder", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (2, "b"), (3, "b")`)

		client := tc.Client()

		{
			res, _, err := client.From("test").Select("*", "", false).
				Order("id", &postgrest.OrderOpts{
					Ascending: true,
				}).
				Execute()
			assert.NoError(t, err)

			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 1, rv[0]["id"])
			assert.EqualValues(t, 2, rv[1]["id"])
			assert.EqualValues(t, 3, rv[2]["id"])
		}

		{
			res, _, err := client.From("test").Select("*", "", false).
				Order("id", &postgrest.OrderOpts{
					Ascending: false,
				}).
				Execute()
			assert.NoError(t, err)

			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 3, rv[0]["id"])
			assert.EqualValues(t, 2, rv[1]["id"])
			assert.EqualValues(t, 1, rv[2]["id"])
		}

		{
			res, _, err := client.From("test").Select("*", "", false).
				Order("s", &postgrest.OrderOpts{
					Ascending: true,
				}).
				Order("id", &postgrest.OrderOpts{
					Ascending: false,
				}).
				Execute()
			assert.NoError(t, err)

			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 1, rv[0]["id"])
			assert.EqualValues(t, 3, rv[1]["id"])
			assert.EqualValues(t, 2, rv[2]["id"])
		}
	})

	t.Run("SelectPagination", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")
		var ps []string
		for i := 0; i < 10; i++ {
			ps = append(ps, fmt.Sprintf("(%d)", i+1))
		}
		tc.ExecuteSQL(t, fmt.Sprintf(`INSERT INTO test (id) VALUES %s`, strings.Join(ps, ", ")))

		client := tc.Client()

		{
			res, _, err := client.From("test").Select("*", "", false).
				Limit(3, "").
				Order("id", &postgrest.OrderOpts{Ascending: true}).
				Execute()
			assert.NoError(t, err)

			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 1, rv[0]["id"])
			assert.EqualValues(t, 2, rv[1]["id"])
			assert.EqualValues(t, 3, rv[2]["id"])
		}

		{
			res, _, err := client.From("test").Select("*", "", false).
				Range(3, 5, "").
				Order("id", &postgrest.OrderOpts{Ascending: true}).
				Execute()
			assert.NoError(t, err)

			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 4, rv[0]["id"])
			assert.EqualValues(t, 5, rv[1]["id"])
			assert.EqualValues(t, 6, rv[2]["id"])
		}

		{
			req := tc.NewRequest(t, http.MethodGet, "test", nil)
			req.Header.Set("Range", "3-5")
			resp := tc.ExecuteRequest(t, req)
			defer resp.Body.Close()

			res, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 4, rv[0]["id"])
			assert.EqualValues(t, 5, rv[1]["id"])
			assert.EqualValues(t, 6, rv[2]["id"])
			assert.Equal(t, resp.Header.Get("Content-Range"), "3-5/*")
		}

		{
			req := tc.NewRequest(t, http.MethodGet, "test", nil)
			req.Header.Set("Range", "7-")
			resp := tc.ExecuteRequest(t, req)
			defer resp.Body.Close()

			res, err := io.ReadAll(resp.Body)
			assert.NoError(t, err)
			var rv []map[string]interface{}
			tc.DecodeResult(t, res, &rv)
			assert.Len(t, rv, 3)
			assert.EqualValues(t, 8, rv[0]["id"])
			assert.EqualValues(t, 9, rv[1]["id"])
			assert.EqualValues(t, 10, rv[2]["id"])
			assert.Equal(t, resp.Header.Get("Content-Range"), "7-/*")
		}
	})

	t.Run("SelectView", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int)")
		tc.ExecuteSQL(t, `INSERT INTO test (id) VALUES (1), (1), (1)`)
		tc.ExecuteSQL(t, "CREATE VIEW test_view (id) AS SELECT id + 1 FROM test")

		client := tc.Client()
		res, _, err := client.From("test_view").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Len(t, rv, 3)
		for _, row := range rv {
			assert.EqualValues(t, 2, row["id"])
		}
	})
}
