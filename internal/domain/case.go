package domain

import (
	"strings"
	"time"
)

type CaseStatus string

const (
	CaseSubmitted          CaseStatus = "submitted"
	CaseTriaged            CaseStatus = "triaged"
	CaseSurveying          CaseStatus = "surveying"
	CaseStewardshipPlanned CaseStatus = "stewardship_planned"
	CaseExecuting          CaseStatus = "executing"
	CaseVerifying          CaseStatus = "verifying"
	CaseResolved           CaseStatus = "resolved"
	CaseReopened           CaseStatus = "reopened"
)

var caseTransitions = map[CaseStatus]map[CaseStatus]bool{
	CaseSubmitted:          {CaseTriaged: true},
	CaseTriaged:            {CaseSurveying: true},
	CaseSurveying:          {CaseStewardshipPlanned: true},
	CaseStewardshipPlanned: {CaseExecuting: true},
	CaseExecuting:          {CaseVerifying: true},
	CaseVerifying:          {CaseResolved: true, CaseReopened: true},
	CaseResolved:           {CaseReopened: true},
	CaseReopened:           {CaseSurveying: true},
}

type ForestCase struct {
	ID             ID
	ForestSiteID   ID
	ForestParcelID ID
	ReporterID     ID
	OwnerID        *ID
	Title          string
	Description    string
	Status         CaseStatus
	Priority       int
	Version        int64
	SubmittedAt    time.Time
	UpdatedAt      time.Time
	ResolvedAt     *time.Time
	ReopenUntil    *time.Time
}

func NewForestCase(id, forest_siteID, zoneID, reporterID ID, title, description string, priority int, now time.Time) (ForestCase, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if len(title) < 4 || len(title) > 160 {
		return ForestCase{}, FieldError{Field: "title", Message: "must contain 4 to 160 characters"}
	}
	if len(description) < 12 || len(description) > 4000 {
		return ForestCase{}, FieldError{Field: "description", Message: "must contain 12 to 4000 characters"}
	}
	if priority < 1 || priority > 5 {
		return ForestCase{}, FieldError{Field: "priority", Message: "must be between 1 and 5"}
	}
	now = now.UTC()
	return ForestCase{ID: id, ForestSiteID: forest_siteID, ForestParcelID: zoneID, ReporterID: reporterID, Title: title, Description: description, Status: CaseSubmitted, Priority: priority, Version: 1, SubmittedAt: now, UpdatedAt: now}, nil
}

func (c *ForestCase) Transition(next CaseStatus, now time.Time) error {
	if !caseTransitions[c.Status][next] {
		return TransitionError{Entity: "forest_case", From: string(c.Status), To: string(next)}
	}
	c.Status = next
	c.Version++
	c.UpdatedAt = now.UTC()
	if next == CaseResolved {
		resolved := now.UTC()
		reopen := resolved.Add(14 * 24 * time.Hour)
		c.ResolvedAt = &resolved
		c.ReopenUntil = &reopen
	}
	if next == CaseReopened {
		c.ResolvedAt = nil
		c.ReopenUntil = nil
	}
	return nil
}

func (c *ForestCase) Assign(owner ID, expectedVersion int64, now time.Time) error {
	if c.Status != CaseSubmitted && c.Status != CaseTriaged {
		return TransitionError{Entity: "forest_case", From: string(c.Status), To: "assigned"}
	}
	if c.Version != expectedVersion {
		return VersionConflictError{Entity: "forest_case", Expected: expectedVersion, Actual: c.Version}
	}
	if c.OwnerID != nil && *c.OwnerID != owner {
		return ConflictError{Resource: "forest_case", Reason: "already assigned"}
	}
	ownerCopy := owner
	c.OwnerID = &ownerCopy
	if c.Status == CaseSubmitted {
		c.Status = CaseTriaged
	}
	c.Version++
	c.UpdatedAt = now.UTC()
	return nil
}

func (c ForestCase) CanReopen(now time.Time) bool {
	return c.Status == CaseResolved && c.ReopenUntil != nil && !now.UTC().After(c.ReopenUntil.UTC())
}

func (c ForestCase) Clone() ForestCase {
	copyCase := c
	if c.OwnerID != nil {
		owner := *c.OwnerID
		copyCase.OwnerID = &owner
	}
	if c.ResolvedAt != nil {
		resolved := *c.ResolvedAt
		copyCase.ResolvedAt = &resolved
	}
	if c.ReopenUntil != nil {
		reopen := *c.ReopenUntil
		copyCase.ReopenUntil = &reopen
	}
	return copyCase
}

type CaseEvent struct {
	ID        ID
	CaseID    ID
	ActorID   ID
	Type      string
	Payload   map[string]string
	RequestID string
	CreatedAt time.Time
}

func (e CaseEvent) Clone() CaseEvent {
	copyEvent := e
	copyEvent.Payload = make(map[string]string, len(e.Payload))
	for key, value := range e.Payload {
		copyEvent.Payload[key] = value
	}
	return copyEvent
}
