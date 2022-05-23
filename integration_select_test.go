package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelect_SingleTable_NoTable(t *testing.T) {
	tc := createTestContextUsingInMemoryDB(t)
	defer tc.CleanUp(t)

	targetURL := tc.ServerURL()
	targetURL.Path += "/test"
	req, err := http.NewRequest(
		http.MethodGet,
		targetURL.String(),
		nil,
	)
	assert.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	assert.NoError(t, err)
	defer resp.Body.Close()

	var body ServerError
	err = json.NewDecoder(resp.Body).Decode(&body)
	assert.NoError(t, err)

	assert.Equal(t, "no such table: test", body.Message)
}
