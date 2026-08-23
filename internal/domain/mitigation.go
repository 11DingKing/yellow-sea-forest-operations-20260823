package domain

import (
	"math"
	"strings"
	"time"
)

type PlanStatus string

const (
	PlanDraft            PlanStatus = "draft"
	PlanOperatorAccepted PlanStatus = "operator_accepted"
	PlanApproved         PlanStatus = "approved"
	PlanExecuting        PlanStatus = "executing"
	PlanCompleted        PlanStatus = "completed"
	PlanRejected         PlanStatus = "rejected"
)

type StewardshipPlan struct {
	ID             ID
	CaseID         ID
	ForestSurveyID ID
	CreatedBy      ID
	Status         PlanStatus
	Description    string
	CutoffMinute   int
	ExpectedMaxLux float64
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ApprovedAt     *time.Time
}

func NewStewardshipPlan(id, caseID, forest_surveyID, createdBy ID, description string, cutoffMinute int, maxLux float64, now time.Time) (StewardshipPlan, error) {
	description = strings.TrimSpace(description)
	if len(description) < 12 || len(description) > 4000 {
		return StewardshipPlan{}, FieldError{Field: "description", Message: "must contain 12 to 4000 characters"}
	}
	if cutoffMinute < 0 || cutoffMinute >= 24*60 {
		return StewardshipPlan{}, FieldError{Field: "cutoff_minute", Message: "must be within a day"}
	}
	if math.IsNaN(maxLux) || math.IsInf(maxLux, 0) || maxLux <= 0 || maxLux > 1000 {
		return StewardshipPlan{}, FieldError{Field: "expected_max_lux", Message: "must be between 0 and 1000"}
	}
	now = now.UTC()
	return StewardshipPlan{ID: id, CaseID: caseID, ForestSurveyID: forest_surveyID, CreatedBy: createdBy, Status: PlanDraft, Description: description, CutoffMinute: cutoffMinute, ExpectedMaxLux: maxLux, Version: 1, CreatedAt: now, UpdatedAt: now}, nil
}

func (p *StewardshipPlan) AcceptOperator(now time.Time) error {
	if p.Status != PlanDraft {
		return TransitionError{Entity: "stewardship_plan", From: string(p.Status), To: string(PlanOperatorAccepted)}
	}
	p.Status = PlanOperatorAccepted
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *StewardshipPlan) Approve(now time.Time) error {
	if p.Status != PlanOperatorAccepted {
		return TransitionError{Entity: "stewardship_plan", From: string(p.Status), To: string(PlanApproved)}
	}
	p.Status = PlanApproved
	p.Version++
	p.UpdatedAt = now.UTC()
	approved := now.UTC()
	p.ApprovedAt = &approved
	return nil
}

func (p *StewardshipPlan) BeginExecution(now time.Time) error {
	if p.Status != PlanApproved {
		return TransitionError{Entity: "stewardship_plan", From: string(p.Status), To: string(PlanExecuting)}
	}
	p.Status = PlanExecuting
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

func (p *StewardshipPlan) Complete(now time.Time) error {
	if p.Status != PlanExecuting && p.Status != PlanApproved {
		return TransitionError{Entity: "stewardship_plan", From: string(p.Status), To: string(PlanCompleted)}
	}
	p.Status = PlanCompleted
	p.Version++
	p.UpdatedAt = now.UTC()
	return nil
}

type ForestAssetActionType string

const (
	ActionInstallShield  ForestAssetActionType = "install_shield"
	ActionAdjustAngle    ForestAssetActionType = "adjust_angle"
	ActionDisableRow     ForestAssetActionType = "disable_row"
	ActionScheduleCutoff ForestAssetActionType = "schedule_cutoff"
)

type ActionStatus string

const (
	ActionPending   ActionStatus = "pending"
	ActionClaimed   ActionStatus = "claimed"
	ActionCompleted ActionStatus = "completed"
	ActionFailed    ActionStatus = "failed"
)

type ForestAssetAction struct {
	ID             ID
	PlanID         ID
	ForestAssetID  ID
	Type           ForestAssetActionType
	Status         ActionStatus
	TargetValue    string
	WorkerID       *ID
	LeaseExpiresAt *time.Time
	Attempt        int
	LastError      string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (a ForestAssetAction) LeaseActive(now time.Time) bool {
	return a.Status == ActionClaimed && a.LeaseExpiresAt != nil && now.UTC().Before(a.LeaseExpiresAt.UTC())
}

func (a *ForestAssetAction) Fail(reason string, now time.Time) error {
	if a.Status != ActionClaimed {
		return TransitionError{Entity: "forest_asset_action", From: string(a.Status), To: string(ActionFailed)}
	}
	a.Status = ActionFailed
	a.LastError = reason
	a.Version++
	a.UpdatedAt = now.UTC()
	return nil
}

func (a ForestAssetAction) Clone() ForestAssetAction {
	copyAction := a
	if a.WorkerID != nil {
		worker := *a.WorkerID
		copyAction.WorkerID = &worker
	}
	if a.LeaseExpiresAt != nil {
		lease := *a.LeaseExpiresAt
		copyAction.LeaseExpiresAt = &lease
	}
	return copyAction
}
