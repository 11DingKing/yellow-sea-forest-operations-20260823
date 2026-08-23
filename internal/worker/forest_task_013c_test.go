package worker

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"testing"
	"time"
)

type cancellationStore struct{ *fakeJobStore }

func (s *cancellationStore) CompleteJob(ctx context.Context, id, worker domain.ID, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.fakeJobStore.CompleteJob(ctx, id, worker, now)
}
func TestSuccessfulJobCompletesAfterParentCancellation(t *testing.T) {
	job := pendingJob("job_cancel_complete")
	store := &cancellationStore{newFakeJobStore(job)}
	parent, cancel := context.WithCancel(context.Background())
	runner, e := New(store, handlerFunc(func(context.Context, domain.OutboxJob) error { cancel(); return nil }), clock.NewManual(workerNow), discardLogger(), Config{BatchSize: 1, Concurrency: 1})
	if e != nil {
		t.Fatal(e)
	}
	_ = runner.runBatch(parent)
	stored := store.snapshot(job.ID)
	if stored.Status != domain.JobDone {
		t.Fatalf("job was not completed after parent cancellation: %#v", stored)
	}
}
