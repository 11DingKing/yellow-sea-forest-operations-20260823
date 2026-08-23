package repository

import (
	"context"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

type UserRepository interface {
	CreateUser(context.Context, domain.User) error
	UserByID(context.Context, domain.ID) (domain.User, error)
	UserByEmail(context.Context, string) (domain.User, error)
	CreateSession(context.Context, domain.Session) error
	SessionByTokenHash(context.Context, []byte) (domain.Session, error)
	RevokeSession(context.Context, domain.ID, time.Time) error
	DeleteExpiredSessions(context.Context, time.Time, int) (int, error)
}

type ForestSiteRepository interface {
	CreateForestSite(context.Context, domain.ForestSite) error
	ForestSiteByID(context.Context, domain.ID) (domain.ForestSite, error)
	UpdateForestSite(context.Context, domain.ForestSite, int64) error
	CreateZone(context.Context, domain.ForestParcel) error
	ZoneByID(context.Context, domain.ID) (domain.ForestParcel, error)
	CreateForestAsset(context.Context, domain.ForestAsset) error
	ForestAssetByID(context.Context, domain.ID) (domain.ForestAsset, error)
	ListForestAssets(context.Context, domain.ID) ([]domain.ForestAsset, error)
	UpdateForestAsset(context.Context, domain.ForestAsset, int64) error
}

type CaseRepository interface {
	CreateCase(context.Context, domain.ForestCase) error
	CaseByID(context.Context, domain.ID) (domain.ForestCase, error)
	UpdateCase(context.Context, domain.ForestCase, int64) error
	AppendCaseEvent(context.Context, domain.CaseEvent) error
	ListCaseEvents(context.Context, domain.ID) ([]domain.CaseEvent, error)
	ListCases(context.Context, domain.CaseFilter, domain.PageRequest) (domain.Page[domain.ForestCase], error)
}

type ForestSurveyRepository interface {
	CreateForestSurvey(context.Context, domain.ForestSurvey) error
	ForestSurveyByID(context.Context, domain.ID) (domain.ForestSurvey, error)
	OpenForestSurveyForCase(context.Context, domain.ID) (domain.ForestSurvey, error)
	UpdateForestSurvey(context.Context, domain.ForestSurvey, int64) error
	AddReading(context.Context, domain.ForestSurveyReading) error
	ListReadings(context.Context, domain.ID) ([]domain.ForestSurveyReading, error)
}

type StewardshipRepository interface {
	CreatePlan(context.Context, domain.StewardshipPlan) error
	PlanByID(context.Context, domain.ID) (domain.StewardshipPlan, error)
	PlanForCase(context.Context, domain.ID) (domain.StewardshipPlan, error)
	UpdatePlan(context.Context, domain.StewardshipPlan, int64) error
	CreateActions(context.Context, []domain.ForestAssetAction) error
	ActionByID(context.Context, domain.ID) (domain.ForestAssetAction, error)
	ListActions(context.Context, domain.ID) ([]domain.ForestAssetAction, error)
	ClaimAction(context.Context, domain.ID, domain.ID, time.Time, time.Time) (domain.ForestAssetAction, error)
	CompleteAction(context.Context, domain.ID, domain.ID, int64, time.Time) error
}

type InspectionRepository interface {
	CreateInspection(context.Context, domain.InspectionRound) error
	InspectionByID(context.Context, domain.ID) (domain.InspectionRound, error)
	OpenInspectionForCase(context.Context, domain.ID) (domain.InspectionRound, error)
	UpdateInspection(context.Context, domain.InspectionRound, int64) error
}

type JobRepository interface {
	EnqueueJob(context.Context, domain.OutboxJob) error
	ClaimJobs(context.Context, domain.ID, int, time.Time, time.Time) ([]domain.OutboxJob, error)
	CompleteJob(context.Context, domain.ID, domain.ID, time.Time) error
	CompleteJobByLease(context.Context, domain.ID, domain.ID, int, time.Time) error
	RetryJob(context.Context, domain.ID, domain.ID, string, time.Time, time.Time) error
	DeadJob(context.Context, domain.ID, domain.ID, string, time.Time) error
}

type AuditRepository interface {
	AppendAudit(context.Context, domain.AuditEvent) error
	ListAudit(context.Context, string, domain.ID, int) ([]domain.AuditEvent, error)
}

type IdempotencyRecord struct {
	Scope        string
	Key          string
	RequestHash  string
	ResourceID   domain.ID
	ResponseCode int
	ResponseBody []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type IdempotencyRepository interface {
	IdempotencyRecord(context.Context, string, string) (IdempotencyRecord, error)
	CreateIdempotencyRecord(context.Context, IdempotencyRecord) error
}

type Store interface {
	UserRepository
	ForestSiteRepository
	CaseRepository
	ForestSurveyRepository
	StewardshipRepository
	InspectionRepository
	JobRepository
	AuditRepository
	IdempotencyRepository
}

type UnitOfWork interface {
	WithinTx(context.Context, func(Store) error) error
	Store() Store
}

type HealthRepository interface {
	Ping(context.Context) error
	SchemaVersion(context.Context) (int, error)
}
