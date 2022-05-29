package main

import (
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
