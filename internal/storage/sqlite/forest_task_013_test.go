package sqlite

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"testing"
	"time"
)

func TestExpiredWorkerCannotCompleteNewLease(t *testing.T) {
	db, _ := openTestDatabase(t)
	old := domain.ID("worker_old")
	now := storageNow
	job := domain.OutboxJob{ID: "job_expired_worker", Topic: "case.submitted", AggregateID: "case_001", Payload: []byte("{}"), Status: domain.JobRunning, AvailableAt: now, Attempt: 2, MaxAttempts: 3, WorkerID: &old, LeaseExpiresAt: func() *time.Time { v := now.Add(-time.Minute); return &v }(), CreatedAt: now, UpdatedAt: now}
	if err := db.Store().EnqueueJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if err := db.Store().CompleteJobByLease(context.Background(), job.ID, old, 2, now); err == nil {
		t.Fatal("expired worker completed a lease")
	}
}
