//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tobioyinlola/RiskOS/backend/internal/jobs"
)

// TestNoopJobRunsEndToEnd is acceptance criterion 6: a job enqueued through the jobs interface is
// persisted, fetched and executed by a worker against real PostgreSQL.
func TestNoopJobRunsEndToEnd(t *testing.T) {
	executed := make(chan jobs.NoopArgs, 1)
	registry := jobs.NewRegistry()
	jobs.Register(registry, func(_ context.Context, args jobs.NoopArgs) error {
		executed <- args
		return nil
	})

	queue, err := jobs.NewQueue(shared.pool, registry, jobs.QueueConfig{Concurrency: 2, JobTimeout: 10 * time.Second}, shared.logger)
	require.NoError(t, err)
	require.NoError(t, queue.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, queue.Stop(ctx))
	})

	require.NoError(t, queue.Enqueue(t.Context(), jobs.NoopArgs{Reason: "integration test"}))

	select {
	case args := <-executed:
		require.Equal(t, "integration test", args.Reason)
	case <-time.After(15 * time.Second):
		t.Fatal("noop job was not executed within 15s")
	}
}
