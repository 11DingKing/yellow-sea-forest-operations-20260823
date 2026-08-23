package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type Handler interface {
	Handle(context.Context, domain.OutboxJob) error
}

type PermanentError struct {
	Cause error
}

func (e PermanentError) Error() string { return "permanent job failure: " + e.Cause.Error() }
func (e PermanentError) Unwrap() error { return e.Cause }

type Config struct {
	PollInterval time.Duration
	Lease        time.Duration
	JobTimeout   time.Duration
	BatchSize    int
	Concurrency  int
}

func (c Config) normalized() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = 2 * time.Second
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.JobTimeout <= 0 || c.JobTimeout >= c.Lease {
		c.JobTimeout = c.Lease * 3 / 4
	}
	if c.BatchSize < 1 || c.BatchSize > 100 {
		c.BatchSize = 20
	}
	if c.Concurrency < 1 || c.Concurrency > 32 {
		c.Concurrency = 4
	}
	return c
}

type Runner struct {
	store    repository.JobRepository
	handler  Handler
	clock    clock.Clock
	logger   *slog.Logger
	workerID domain.ID
	config   Config
}

func New(store repository.JobRepository, handler Handler, taskClock clock.Clock, logger *slog.Logger, config Config) (*Runner, error) {
	if store == nil || handler == nil || taskClock == nil {
		return nil, fmt.Errorf("worker dependencies are required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	id, err := domain.NewID("worker")
	if err != nil {
		return nil, err
	}
	return &Runner{store: store, handler: handler, clock: taskClock, logger: logger, workerID: id, config: config.normalized()}, nil
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := r.runBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.ErrorContext(ctx, "outbox batch failed", "worker_id", r.workerID, "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) runBatch(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := r.clock.Now()
	jobs, err := r.store.ClaimJobs(ctx, r.workerID, r.config.BatchSize, now, now.Add(r.config.Lease))
	if err != nil {
		return fmt.Errorf("claim jobs: %w", err)
	}
	if len(jobs) == 0 {
		return nil
	}
	semaphore := make(chan struct{}, r.config.Concurrency)
	var wait sync.WaitGroup
	for _, job := range jobs {
		job := job.Clone()
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			defer func() { <-semaphore }()
			r.process(ctx, job)
		}()
	}
	wait.Wait()
	return nil
}

func (r *Runner) process(parent context.Context, job domain.OutboxJob) {
	ctx, cancel := context.WithTimeout(parent, r.config.JobTimeout)
	defer cancel()
	started := r.clock.Now()
	err := safelyHandle(r.handler, ctx, job.Clone())
	now := r.clock.Now()
	if err == nil {
		if finishErr := r.store.CompleteJob(parent, job.ID, r.workerID, now); finishErr != nil {
			r.logger.WarnContext(parent, "complete outbox job failed", "job_id", job.ID, "topic", job.Topic, "error", finishErr)
			return
		}
		r.logger.InfoContext(parent, "outbox job completed", "job_id", job.ID, "topic", job.Topic, "attempt", job.Attempt, "duration", now.Sub(started))
		return
	}
	var permanent PermanentError
	if errors.As(err, &permanent) || job.Attempt >= job.MaxAttempts {
		if finishErr := r.store.DeadJob(parent, job.ID, r.workerID, err.Error(), now); finishErr != nil {
			r.logger.WarnContext(parent, "dead-letter outbox job failed", "job_id", job.ID, "error", finishErr)
		}
		return
	}
	availableAt := now.Add(domain.RetryDelay(job.Attempt))
	if retryErr := r.store.RetryJob(parent, job.ID, r.workerID, err.Error(), availableAt, now); retryErr != nil {
		r.logger.WarnContext(parent, "retry outbox job failed", "job_id", job.ID, "error", retryErr)
		return
	}
	r.logger.WarnContext(parent, "outbox job scheduled for retry", "job_id", job.ID, "topic", job.Topic, "attempt", job.Attempt, "available_at", availableAt, "error", err)
}

func safelyHandle(handler Handler, ctx context.Context, job domain.OutboxJob) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("outbox handler panic: %v", recovered)
		}
	}()
	return handler.Handle(ctx, job)
}
