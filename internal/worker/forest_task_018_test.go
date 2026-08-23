package worker

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"testing"
	"time"
)

func TestWorkerStopsWhenParentIsCancelled(t *testing.T) {
	job := pendingJob("job_shutdown_cancel")
	store := newFakeJobStore(job)
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	runner, e := New(store, handlerFunc(func(ctx context.Context, _ domain.OutboxJob) error { close(started); <-ctx.Done(); return ctx.Err() }), clock.NewManual(workerNow), discardLogger(), Config{BatchSize: 1, Concurrency: 1, Lease: time.Second, JobTimeout: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- runner.runBatch(parent) }()
	<-started
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("handler ignored cancellation")
	}
}
