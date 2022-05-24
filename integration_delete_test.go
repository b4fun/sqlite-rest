package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDelete_SingleTable(t *testing.T) {
	t.Run("NoTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From("test").Delete("", "").Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table: test")
	})

	t.Run("DeleteFromEmptyTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")

		client := tc.Client()
		_, _, err := client.From("test").Delete("", "").Execute()
		assert.NoError(t, err)
	})

	t.Run("DeleteFromNonEmptyTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (1, "a"), (1, "a")`)

		client := tc.Client()
		_, _, err := client.From("test").Delete("", "").Execute()
		assert.NoError(t, err)

		res, _, err := client.From("test").Select("id", "", false).
			Execute()
		assert.NoError(t, err)

		var rv []map[string]interface{}
		tc.DecodeResult(t, res, &rv)
		assert.Empty(t, rv)
	})

	t.Run("DeleteWithFilter", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
		tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (2, "a"), (3, "a")`)

		client := tc.Client()
		_, _, err := client.From("test").Delete("", "").
			Gt("id", "1").
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
}
