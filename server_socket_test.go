package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
)

func TestServerWithUnixSocket(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	socketPath := filepath.Join(dir, "sqlite-rest.sock")

	dbPath := filepath.Join(dir, "test.db")
	db, err := sqlx.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec("CREATE TABLE test (id int)")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO test (id) VALUES (1)`)
	if err != nil {
		t.Fatal(err)
	}

	serverOpts := &ServerOptions{
		Logger:     createTestLogger(t).WithName("test"),
		Queryer:    db,
		Execer:     db,
		SocketPath: socketPath,
	}
	serverOpts.AuthOptions.disableAuth = true
	serverOpts.SecurityOptions.EnabledTableOrViews = []string{"test"}

	server, err := NewServer(serverOpts)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		server.Start(done)
		close(serverDone)
	}()

	assert.Eventually(t, func() bool {
		_, err := os.Stat(socketPath)
		return err == nil
	}, 5*time.Second, 100*time.Millisecond)

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
	}

	req, err := http.NewRequest(http.MethodGet, "http://unix/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var rows []map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&rows)
	assert.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.EqualValues(t, 1, rows[0]["id"])

	close(done)
	assert.Eventually(t, func() bool {
		select {
		case <-serverDone:
			return true
		default:
			return false
		}
	}, 2*time.Second, 50*time.Millisecond)

	assert.Eventually(t, func() bool {
		_, err := os.Stat(socketPath)
		return errors.Is(err, os.ErrNotExist)
	}, 5*time.Second, 100*time.Millisecond)
}
