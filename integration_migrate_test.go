package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMigration(t *testing.T) {
	t.Run("empty migrations", func(t *testing.T) {
		tc := NewMigrationTestContext(t, nil)
		defer tc.CleanUp(t)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := tc.Migrator().Up(ctx)
		assert.NoError(t, err)
	})

	t.Run("apply migrations", func(t *testing.T) {
		tc := NewMigrationTestContext(t, map[string]string{
			"1_test.up.sql":   `create table test (id int);`,
			"1_test.down.sql": `drop table test;`,
		})
		defer tc.CleanUp(t)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := tc.Migrator().Up(ctx)
		assert.NoError(t, err)

		t.Log("rerunning migrations")
		err = tc.Migrator().Up(ctx)
		assert.NoError(t, err)
	})

	t.Run("failed migrations", func(t *testing.T) {
		tc := NewMigrationTestContext(t, map[string]string{
			"1_test.up.sql":   `create table test invalid sql;`,
			"1_test.down.sql": `drop table test;`,
		})
		defer tc.CleanUp(t)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := tc.Migrator().Up(ctx)
		assert.Error(t, err)
	})
}
