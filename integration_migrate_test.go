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

		err := tc.Migrator().Up(ctx, migrationStepAll)
		assert.NoError(t, err)

		err = tc.Migrator().Down(ctx, migrationStepAll)
		assert.NoError(t, err)
	})

	t.Run("apply all migrations", func(t *testing.T) {
		tc := NewMigrationTestContext(t, map[string]string{
			"1_test.up.sql":   `create table test (id int);`,
			"1_test.down.sql": `drop table test;`,
		})
		defer tc.CleanUp(t)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		t.Log("up")
		{
			err := tc.Migrator().Up(ctx, migrationStepAll)
			assert.NoError(t, err)

			t.Log("rerunning migrations")
			err = tc.Migrator().Up(ctx, migrationStepAll)
			assert.NoError(t, err)
		}

		t.Log("down")
		{
			err := tc.Migrator().Down(ctx, migrationStepAll)
			assert.NoError(t, err)

			t.Log("rerunning migrations")
			err = tc.Migrator().Down(ctx, migrationStepAll)
			assert.NoError(t, err)
		}
	})

	t.Run("apply migrations by step", func(t *testing.T) {
		tc := NewMigrationTestContext(t, map[string]string{
			"1_test.up.sql":    `create table test (id int);`,
			"1_test.down.sql":  `drop table test;`,
			"2_test2.up.sql":   `create table test2 (id int);`,
			"2_test2.down.sql": `drop table test2;`,
		})
		defer tc.CleanUp(t)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		t.Log("up 1 step (current step = 0)")
		{
			err := tc.Migrator().Up(ctx, 1)
			assert.NoError(t, err)
		}
		t.Log("up 1 step (current step = 1)")
		{
			err := tc.Migrator().Up(ctx, 1)
			assert.NoError(t, err)
		}
		t.Log("up 1 step (current step = 2)")
		{
			err := tc.Migrator().Up(ctx, 1)
			assert.Error(t, err)
		}

		t.Log("down 1 step (current step = 2)")
		{
			err := tc.Migrator().Down(ctx, 1)
			assert.NoError(t, err)
		}
		t.Log("down 1 step (current step = 1)")
		{
			err := tc.Migrator().Down(ctx, 1)
			assert.NoError(t, err)
		}
		t.Log("down 1 step (current step = 0)")
		{
			err := tc.Migrator().Down(ctx, 1)
			assert.Error(t, err)
		}
	})

	t.Run("failed migrations", func(t *testing.T) {
		t.Run("up", func(t *testing.T) {
			tc := NewMigrationTestContext(t, map[string]string{
				"1_test.up.sql":   `create table test invalid sql;`,
				"1_test.down.sql": `drop table test;`,
			})
			defer tc.CleanUp(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := tc.Migrator().Up(ctx, migrationStepAll)
			assert.Error(t, err)
		})

		t.Run("down", func(t *testing.T) {
			tc := NewMigrationTestContext(t, map[string]string{
				"1_test.up.sql":   `create table test (id int);`,
				"1_test.down.sql": `drop table invalid sql;`,
			})
			defer tc.CleanUp(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := tc.Migrator().Up(ctx, migrationStepAll)
			assert.NoError(t, err)

			err = tc.migrator.Down(ctx, migrationStepAll)
			assert.Error(t, err)
		})
	})
}
