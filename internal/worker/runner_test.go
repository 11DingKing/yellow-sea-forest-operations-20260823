package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

var workerNow = time.Date(2026, 8, 20, 13, 0, 0, 0, time.UTC)

type fakeJobStore struct {
	mu        sync.Mutex
	jobs      map[domain.ID]domain.OutboxJob
	completed []domain.ID
	retried   []domain.ID
	dead      []domain.ID
}

func newFakeJobStore(jobs ...domain.OutboxJob) *fakeJobStore {
	store := &fakeJobStore{jobs: make(map[domain.ID]domain.OutboxJob)}
	for _, job := range jobs {
		store.jobs[job.ID] = job.Clone()
	}
	return store
}

func (s *fakeJobStore) EnqueueJob(_ context.Context, job domain.OutboxJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.jobs[job.ID]; exists {
		return domain.ErrConflict
	}
	s.jobs[job.ID] = job.Clone()
	return nil
}

func (s *fakeJobStore) ClaimJobs(ctx context.Context, workerID domain.ID, limit int, now, leaseUntil time.Time) ([]domain.OutboxJob, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]domain.OutboxJob, 0, limit)
	for id, job := range s.jobs {
		if len(result) == limit {
			break
		}
		if !job.Claimable(now) {
			continue
		}
		owner := workerID
		lease := leaseUntil
		job.Status = domain.JobRunning
		job.WorkerID = &owner
		job.LeaseExpiresAt = &lease
		job.Attempt++
		job.UpdatedAt = now
		s.jobs[id] = job.Clone()
		result = append(result, job.Clone())
	}
	return result, nil
}

func (s *fakeJobStore) CompleteJob(_ context.Context, id, workerID domain.ID, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.jobs[id]
	if !exists || job.WorkerID == nil || *job.WorkerID != workerID || job.Status != domain.JobRunning {
		return domain.ErrLeaseLost
	}
	job.Status, job.WorkerID, job.LeaseExpiresAt, job.UpdatedAt = domain.JobDone, nil, nil, now
	s.jobs[id] = job
	s.completed = append(s.completed, id)
	return nil
}

func (s *fakeJobStore) RetryJob(_ context.Context, id, workerID domain.ID, message string, availableAt, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.jobs[id]
	if !exists || job.WorkerID == nil || *job.WorkerID != workerID {
		return domain.ErrLeaseLost
	}
	job.Status, job.WorkerID, job.LeaseExpiresAt = domain.JobPending, nil, nil
	job.LastError, job.AvailableAt, job.UpdatedAt = message, availableAt, now
	s.jobs[id] = job
	s.retried = append(s.retried, id)
	return nil
}

func (s *fakeJobStore) DeadJob(_ context.Context, id, workerID domain.ID, message string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.jobs[id]
	if !exists || job.WorkerID == nil || *job.WorkerID != workerID {
		return domain.ErrLeaseLost
	}
	job.Status, job.WorkerID, job.LeaseExpiresAt = domain.JobDead, nil, nil
	job.LastError, job.UpdatedAt = message, now
	s.jobs[id] = job
	s.dead = append(s.dead, id)
	return nil
}

func (s *fakeJobStore) snapshot(id domain.ID) domain.OutboxJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jobs[id].Clone()
}

type handlerFunc func(context.Context, domain.OutboxJob) error

func (f handlerFunc) Handle(ctx context.Context, job domain.OutboxJob) error { return f(ctx, job) }

func pendingJob(id domain.ID) domain.OutboxJob {
	return domain.OutboxJob{ID: id, Topic: "case.submitted", AggregateID: "case_001", Payload: []byte(`{"case_id":"case_001","zone_id":"zone_001"}`), Status: domain.JobPending, AvailableAt: workerNow, MaxAttempts: 3, CreatedAt: workerNow, UpdatedAt: workerNow}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestConfigNormalization(t *testing.T) {
	got := (Config{}).normalized()
	if got.PollInterval != 2*time.Second || got.Lease != 30*time.Second || got.JobTimeout != 22500*time.Millisecond || got.BatchSize != 20 || got.Concurrency != 4 {
		t.Fatalf("normalized defaults = %#v", got)
	}
	input := Config{PollInterval: time.Second, Lease: 40 * time.Second, JobTimeout: 10 * time.Second, BatchSize: 50, Concurrency: 8}
	if got := input.normalized(); got != input {
		t.Fatalf("valid config changed: got=%#v want=%#v", got, input)
	}
	invalid := Config{Lease: 5 * time.Second, JobTimeout: 8 * time.Second, BatchSize: 101, Concurrency: 33}
	got = invalid.normalized()
	if got.JobTimeout >= got.Lease || got.BatchSize != 20 || got.Concurrency != 4 {
		t.Fatalf("invalid config was not normalized: %#v", got)
	}
}

func TestRunnerCompletesSuccessfulJob(t *testing.T) {
	job := pendingJob("job_success")
	store := newFakeJobStore(job)
	manual := clock.NewManual(workerNow)
	var handled domain.OutboxJob
	runner, err := New(store, handlerFunc(func(_ context.Context, job domain.OutboxJob) error {
		handled = job.Clone()
		return nil
	}), manual, discardLogger(), Config{BatchSize: 10, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := store.snapshot(job.ID)
	if handled.ID != job.ID || handled.Attempt != 1 {
		t.Fatalf("handled job = %#v", handled)
	}
	if stored.Status != domain.JobDone || stored.WorkerID != nil || len(store.completed) != 1 {
		t.Fatalf("stored job = %#v, completed=%#v", stored, store.completed)
	}
}

func TestRunnerSchedulesTransientFailureWithBackoff(t *testing.T) {
	job := pendingJob("job_retry")
	store := newFakeJobStore(job)
	manual := clock.NewManual(workerNow)
	runner, err := New(store, handlerFunc(func(context.Context, domain.OutboxJob) error {
		return errors.New("notification endpoint unavailable")
	}), manual, discardLogger(), Config{BatchSize: 10, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := store.snapshot(job.ID)
	if stored.Status != domain.JobPending || stored.LastError != "notification endpoint unavailable" {
		t.Fatalf("retried job = %#v", stored)
	}
	if !stored.AvailableAt.Equal(workerNow.Add(time.Second)) || len(store.retried) != 1 {
		t.Fatalf("retry time = %s, retried=%#v", stored.AvailableAt, store.retried)
	}
	manual.Advance(2 * time.Second)
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored = store.snapshot(job.ID)
	if stored.Attempt != 2 || !stored.AvailableAt.Equal(manual.Now().Add(2*time.Second)) {
		t.Fatalf("second retry = %#v", stored)
	}
}

func TestRunnerDeadLettersPermanentFailure(t *testing.T) {
	job := pendingJob("job_permanent")
	store := newFakeJobStore(job)
	runner, err := New(store, handlerFunc(func(context.Context, domain.OutboxJob) error {
		return PermanentError{Cause: errors.New("malformed payload")}
	}), clock.NewManual(workerNow), discardLogger(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := store.snapshot(job.ID)
	if stored.Status != domain.JobDead || stored.LastError != "permanent job failure: malformed payload" || len(store.dead) != 1 {
		t.Fatalf("dead job = %#v, dead=%#v", stored, store.dead)
	}
}

func TestRunnerDeadLettersAtMaximumAttempt(t *testing.T) {
	job := pendingJob("job_attempts")
	job.Attempt = 2
	job.MaxAttempts = 3
	store := newFakeJobStore(job)
	runner, err := New(store, handlerFunc(func(context.Context, domain.OutboxJob) error {
		return errors.New("still unavailable")
	}), clock.NewManual(workerNow), discardLogger(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := store.snapshot(job.ID)
	if stored.Attempt != 3 || stored.Status != domain.JobDead || len(store.retried) != 0 {
		t.Fatalf("maximum-attempt job = %#v", stored)
	}
}

func TestRunnerRecoversHandlerPanicAndRetriesJob(t *testing.T) {
	job := pendingJob("job_panic")
	store := newFakeJobStore(job)
	runner, err := New(store, handlerFunc(func(context.Context, domain.OutboxJob) error {
		panic("unexpected handler state")
	}), clock.NewManual(workerNow), discardLogger(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := store.snapshot(job.ID)
	if stored.Status != domain.JobPending || !strings.Contains(stored.LastError, "unexpected handler state") || len(store.retried) != 1 {
		t.Fatalf("panic recovery job = %#v", stored)
	}
}

func TestRunnerRespectsConcurrencyLimit(t *testing.T) {
	jobs := make([]domain.OutboxJob, 12)
	for index := range jobs {
		jobs[index] = pendingJob(domain.ID("job_concurrent_" + string(rune('a'+index))))
	}
	store := newFakeJobStore(jobs...)
	var active atomic.Int32
	var maximum atomic.Int32
	gate := make(chan struct{})
	handler := handlerFunc(func(ctx context.Context, _ domain.OutboxJob) error {
		current := active.Add(1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		select {
		case <-gate:
		case <-ctx.Done():
			active.Add(-1)
			return ctx.Err()
		}
		active.Add(-1)
		return nil
	})
	runner, err := New(store, handler, clock.NewManual(workerNow), discardLogger(), Config{BatchSize: len(jobs), Concurrency: 3, Lease: time.Second, JobTimeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- runner.runBatch(context.Background()) }()
	deadline := time.After(time.Second)
	for maximum.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("maximum concurrency reached %d", maximum.Load())
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 3 {
		t.Fatalf("maximum concurrency = %d, want 3", maximum.Load())
	}
	if len(store.completed) != len(jobs) {
		t.Fatalf("completed jobs = %d, want %d", len(store.completed), len(jobs))
	}
}

func TestRunnerPassesDeadlineToHandler(t *testing.T) {
	job := pendingJob("job_timeout")
	store := newFakeJobStore(job)
	handler := handlerFunc(func(ctx context.Context, _ domain.OutboxJob) error {
		<-ctx.Done()
		return ctx.Err()
	})
	runner, err := New(store, handler, clock.NewManual(workerNow), discardLogger(), Config{Lease: 100 * time.Millisecond, JobTimeout: 10 * time.Millisecond, Concurrency: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runBatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored := store.snapshot(job.ID)
	if stored.Status != domain.JobPending || stored.LastError != context.DeadlineExceeded.Error() {
		t.Fatalf("timed-out job = %#v", stored)
	}
}

type recordingSink struct {
	mu            sync.Mutex
	notifications []Notification
	err           error
}

func (s *recordingSink) Deliver(ctx context.Context, notification Notification) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notifications = append(s.notifications, notification)
	return s.err
}

func TestDispatcherBuildsBusinessNotifications(t *testing.T) {
	tests := []struct {
		name       string
		topic      string
		payload    string
		recipients int
		subject    string
	}{
		{name: "case", topic: "case.submitted", payload: `{"case_id":"case_001","zone_id":"zone_001"}`, recipients: 1, subject: "New light-impact complaint"},
		{name: "forest_survey", topic: "forest_survey.published", payload: `{"case_id":"case_001","forest_survey_id":"forest_survey_001","summary_lux":26.5}`, recipients: 2, subject: "Night survey published"},
		{name: "verification", topic: "verification.completed", payload: `{"case_id":"case_001","verification_id":"verification_001","result":"failed"}`, recipients: 3, subject: "Stewardship verification completed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink := &recordingSink{}
			dispatcher := NewDispatcher(sink, discardLogger())
			job := domain.OutboxJob{ID: "job_001", Topic: test.topic, AggregateID: "case_001", Payload: []byte(test.payload)}
			if err := dispatcher.Handle(context.Background(), job); err != nil {
				t.Fatal(err)
			}
			if len(sink.notifications) != 1 {
				t.Fatalf("notifications = %#v", sink.notifications)
			}
			notification := sink.notifications[0]
			if notification.Subject != test.subject || len(notification.Recipients) != test.recipients || notification.AggregateID != job.AggregateID {
				t.Fatalf("notification = %#v", notification)
			}
		})
	}
}

func TestDispatcherClassifiesMalformedAndUnknownJobsAsPermanent(t *testing.T) {
	dispatcher := NewDispatcher(&recordingSink{}, discardLogger())
	tests := []domain.OutboxJob{
		{ID: "job_empty", Topic: "case.submitted"},
		{ID: "job_malformed", Topic: "forest_survey.published", Payload: []byte(`{"broken"`)},
		{ID: "job_missing_ids", Topic: "case.submitted", Payload: []byte(`{}`)},
		{ID: "job_negative_lux", Topic: "forest_survey.published", Payload: []byte(`{"case_id":"case_001","forest_survey_id":"forest_survey_001","summary_lux":-1}`)},
		{ID: "job_bad_result", Topic: "verification.completed", Payload: []byte(`{"case_id":"case_001","verification_id":"verification_001","result":"unknown"}`)},
		{ID: "job_unknown", Topic: "unknown.topic", Payload: []byte(`{}`)},
	}
	for _, job := range tests {
		t.Run(job.ID.String(), func(t *testing.T) {
			err := dispatcher.Handle(context.Background(), job)
			var permanent PermanentError
			if !errors.As(err, &permanent) {
				t.Fatalf("error = %v, want PermanentError", err)
			}
		})
	}
}

func TestDispatcherPropagatesSinkFailure(t *testing.T) {
	sentinel := errors.New("delivery service unavailable")
	sink := &recordingSink{err: sentinel}
	dispatcher := NewDispatcher(sink, discardLogger())
	job := domain.OutboxJob{ID: "job_sink", Topic: "case.submitted", AggregateID: "case_001", Payload: []byte(`{"case_id":"case_001","zone_id":"zone_001"}`)}
	err := dispatcher.Handle(context.Background(), job)
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want sentinel", err)
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	validStore := newFakeJobStore()
	validHandler := handlerFunc(func(context.Context, domain.OutboxJob) error { return nil })
	validClock := clock.NewManual(workerNow)
	if _, err := New(nil, validHandler, validClock, discardLogger(), Config{}); err == nil {
		t.Fatal("New(nil store) error = nil")
	}
	if _, err := New(validStore, nil, validClock, discardLogger(), Config{}); err == nil {
		t.Fatal("New(nil handler) error = nil")
	}
	if _, err := New(validStore, validHandler, nil, discardLogger(), Config{}); err == nil {
		t.Fatal("New(nil clock) error = nil")
	}
}
