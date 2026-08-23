package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) EnqueueJob(ctx context.Context, job domain.OutboxJob) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO outbox_jobs(id,topic,aggregate_id,payload,status,available_at,attempt,max_attempts,worker_id,lease_expires_at,last_error,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, job.ID, job.Topic, job.AggregateID, job.Payload, job.Status, formatTime(job.AvailableAt), job.Attempt, job.MaxAttempts, nullableID(job.WorkerID), nullableTime(job.LeaseExpiresAt), job.LastError, formatTime(job.CreatedAt), formatTime(job.UpdatedAt))
	return mapError("enqueue outbox job", err)
}

func scanJob(row interface{ Scan(...any) error }) (domain.OutboxJob, error) {
	var job domain.OutboxJob
	var id, aggregateID, status, available, created, updated string
	var worker, lease sql.NullString
	if err := row.Scan(&id, &job.Topic, &aggregateID, &job.Payload, &status, &available, &job.Attempt, &job.MaxAttempts, &worker, &lease, &job.LastError, &created, &updated); err != nil {
		return domain.OutboxJob{}, err
	}
	job.ID, job.AggregateID = domain.ID(id), domain.ID(aggregateID)
	job.Status = domain.JobStatus(status)
	if worker.Valid {
		value := domain.ID(worker.String)
		job.WorkerID = &value
	}
	if lease.Valid {
		value, err := parseTime(lease.String)
		if err != nil {
			return domain.OutboxJob{}, err
		}
		job.LeaseExpiresAt = &value
	}
	var err error
	if job.AvailableAt, err = parseTime(available); err != nil {
		return domain.OutboxJob{}, err
	}
	if job.CreatedAt, err = parseTime(created); err != nil {
		return domain.OutboxJob{}, err
	}
	if job.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.OutboxJob{}, err
	}
	return job.Clone(), nil
}

const selectJob = `SELECT id,topic,aggregate_id,payload,status,available_at,attempt,max_attempts,worker_id,lease_expires_at,last_error,created_at,updated_at FROM outbox_jobs`

func (s *store) ClaimJobs(ctx context.Context, workerID domain.ID, limit int, now, leaseUntil time.Time) ([]domain.OutboxJob, error) {
	if limit < 1 || limit > 100 {
		return nil, domain.FieldError{Field: "limit", Message: "must be between 1 and 100"}
	}
	rows, err := s.queryer.QueryContext(ctx, `
		SELECT id FROM outbox_jobs
		WHERE (status='pending' AND available_at<=?) OR (status='running' AND lease_expires_at<=?)
		ORDER BY available_at,id LIMIT ?`, formatTime(now), formatTime(now), limit)
	if err != nil {
		return nil, mapError("find claimable jobs", err)
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	claimed := make([]domain.OutboxJob, 0, len(ids))
	for _, id := range ids {
		result, updateErr := s.queryer.ExecContext(ctx, `
			UPDATE outbox_jobs SET status='running',worker_id=?,lease_expires_at=?,attempt=attempt+1,updated_at=?
			WHERE id=? AND ((status='pending' AND available_at<=?) OR (status='running' AND lease_expires_at<=?))`, workerID, formatTime(leaseUntil), formatTime(now), id, formatTime(now), formatTime(now))
		if updateErr != nil {
			return nil, mapError("claim outbox job", updateErr)
		}
		count, updateErr := result.RowsAffected()
		if updateErr != nil {
			return nil, updateErr
		}
		if count == 0 {
			continue
		}
		job, scanErr := scanJob(s.queryer.QueryRowContext(ctx, selectJob+` WHERE id=?`, id))
		if scanErr != nil {
			return nil, mapError("read claimed outbox job", scanErr)
		}
		claimed = append(claimed, job)
	}
	return claimed, nil
}

func (s *store) CompleteJob(ctx context.Context, id, workerID domain.ID, now time.Time) error {
	return s.finishOwnedJob(ctx, id, workerID, now, "done", "", now)
}

func (s *store) RetryJob(ctx context.Context, id, workerID domain.ID, message string, availableAt, now time.Time) error {
	return s.finishOwnedJob(ctx, id, workerID, now, "pending", message, availableAt)
}

func (s *store) DeadJob(ctx context.Context, id, workerID domain.ID, message string, now time.Time) error {
	return s.finishOwnedJob(ctx, id, workerID, now, "dead", message, now)
}

func (s *store) finishOwnedJob(ctx context.Context, id, workerID domain.ID, now time.Time, status, message string, availableAt time.Time) error {
	message = strings.TrimSpace(message)
	if len(message) > 2000 {
		message = message[:2000]
	}
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE outbox_jobs SET status=?,available_at=?,worker_id=NULL,lease_expires_at=NULL,last_error=?,updated_at=?
		WHERE id=? AND status='running' AND worker_id=? AND lease_expires_at>?`, status, formatTime(availableAt), message, formatTime(now), id, workerID, formatTime(now))
	if err != nil {
		return mapError("finish outbox job", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish outbox job rows: %w", err)
	}
	if rows != 1 {
		return domain.ErrLeaseLost
	}
	return nil
}
