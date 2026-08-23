package domain

import "time"

type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobDead    JobStatus = "dead"
)

type OutboxJob struct {
	ID             ID
	Topic          string
	AggregateID    ID
	Payload        []byte
	Status         JobStatus
	AvailableAt    time.Time
	Attempt        int
	MaxAttempts    int
	WorkerID       *ID
	LeaseExpiresAt *time.Time
	LastError      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (j OutboxJob) Claimable(now time.Time) bool {
	now = now.UTC()
	if j.Status == JobPending {
		return !j.AvailableAt.After(now)
	}
	return j.Status == JobRunning && j.LeaseExpiresAt != nil && !j.LeaseExpiresAt.After(now)
}

func (j OutboxJob) Clone() OutboxJob {
	copyJob := j
	copyJob.Payload = append([]byte(nil), j.Payload...)
	if j.WorkerID != nil {
		worker := *j.WorkerID
		copyJob.WorkerID = &worker
	}
	if j.LeaseExpiresAt != nil {
		lease := *j.LeaseExpiresAt
		copyJob.LeaseExpiresAt = &lease
	}
	return copyJob
}

func RetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}

type AuditEvent struct {
	ID         ID
	ActorID    ID
	Action     string
	ObjectType string
	ObjectID   ID
	Result     string
	RequestID  string
	Metadata   map[string]string
	CreatedAt  time.Time
}

func (e AuditEvent) Clone() AuditEvent {
	copyEvent := e
	copyEvent.Metadata = make(map[string]string, len(e.Metadata))
	for key, value := range e.Metadata {
		copyEvent.Metadata[key] = value
	}
	return copyEvent
}
