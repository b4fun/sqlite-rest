package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInsert_SingleTable(t *testing.T) {
	t.Run("NoTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From("test").
			Insert(map[string]interface{}{"id": 1}, false, "", "", "").
			Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table: test")
	})

	t.Run("InsertSingleValue", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
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
		tc := createTestContextUsingInMemoryDB(t)
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
}