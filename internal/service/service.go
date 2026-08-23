package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type Service struct {
	uow        repository.UnitOfWork
	clock      clock.Clock
	sessionTTL time.Duration
}

func New(uow repository.UnitOfWork, clock clock.Clock, sessionTTL time.Duration) *Service {
	return &Service{uow: uow, clock: clock, sessionTTL: sessionTTL}
}

type Principal struct {
	User      domain.User
	SessionID domain.ID
}

func requireAction(principal Principal, action string) error {
	if !principal.User.Can(action) {
		return fmt.Errorf("role %s cannot perform %s: %w", principal.User.Role, action, domain.ErrForbidden)
	}
	return nil
}

func requestID(ctx context.Context) string {
	if value, ok := ctx.Value(requestIDKey{}).(string); ok {
		return value
	}
	return "unknown"
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		id = "unknown"
	}
	return context.WithValue(ctx, requestIDKey{}, id)
}

func newAudit(actor domain.ID, action, objectType string, objectID domain.ID, result string, metadata map[string]string, requestID string, now time.Time) (domain.AuditEvent, error) {
	id, err := domain.NewID("audit")
	if err != nil {
		return domain.AuditEvent{}, err
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	return domain.AuditEvent{ID: id, ActorID: actor, Action: action, ObjectType: objectType, ObjectID: objectID, Result: result, RequestID: requestID, Metadata: metadata, CreatedAt: now.UTC()}, nil
}

func newCaseEvent(caseID, actor domain.ID, kind string, payload map[string]string, requestID string, now time.Time) (domain.CaseEvent, error) {
	id, err := domain.NewID("evt")
	if err != nil {
		return domain.CaseEvent{}, err
	}
	if payload == nil {
		payload = map[string]string{}
	}
	return domain.CaseEvent{ID: id, CaseID: caseID, ActorID: actor, Type: kind, Payload: payload, RequestID: requestID, CreatedAt: now.UTC()}, nil
}

func newJob(topic string, aggregate domain.ID, payload any, now time.Time) (domain.OutboxJob, error) {
	id, err := domain.NewID("job")
	if err != nil {
		return domain.OutboxJob{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return domain.OutboxJob{}, fmt.Errorf("encode outbox payload: %w", err)
	}
	return domain.OutboxJob{ID: id, Topic: topic, AggregateID: aggregate, Payload: encoded, Status: domain.JobPending, AvailableAt: now.UTC(), MaxAttempts: 6, CreatedAt: now.UTC(), UpdatedAt: now.UTC()}, nil
}
