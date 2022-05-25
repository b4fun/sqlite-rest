package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpdate_SingleTable(t *testing.T) {
	t.Run("NoTable", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
		defer tc.CleanUp(t)

		client := tc.Client()
		_, _, err := client.From("test").Update(map[string]interface{}{"id": 1}, "", "1").
			Execute()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no such table: test")
	})

	t.Run("UpdateRecords", func(t *testing.T) {
		tc := createTestContextUsingInMemoryDB(t)
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
		tc := createTestContextUsingInMemoryDB(t)
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
}