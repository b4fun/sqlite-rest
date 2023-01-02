package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMetricsServer_monitorDatabaseSize(t *testing.T) {
	t.Parallel()

	tc := createTestContextWithHMACTokenAuth(t)
	defer tc.CleanUp(t)

	tc.ExecuteSQL(t, "CREATE TABLE test (id int, s text)")
	tc.ExecuteSQL(t, `INSERT INTO test (id, s) VALUES (1, "a"), (1, "a"), (1, "a")`)

	metricsServer, err := NewMetricsServer(MetricsServerOptions{
		Logger:  createTestLogger(t).WithName("test"),
		Addr:    ":8081",
		Queryer: tc.DB(),
	})
	assert.NoError(t, err)

	done := make(chan struct{})
	observeFinish := make(chan struct{})

	go metricsServer.monitorDatabaseSize(done, func(sizeInBytes float64) {
		close(observeFinish)

		assert.True(t, sizeInBytes > 0)
	})

	time.Sleep(100 * time.Millisecond)
	close(done)
	<-observeFinish
}
