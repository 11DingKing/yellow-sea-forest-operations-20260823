package domain

import (
	"math"
	"strings"
	"time"
)

type InspectionStatus string

const (
	InspectionOpen   InspectionStatus = "open"
	InspectionPassed InspectionStatus = "passed"
	InspectionFailed InspectionStatus = "failed"
)

type InspectionRound struct {
	ID             ID
	CaseID         ID
	PlanID         ID
	InspectorID    ID
	Status         InspectionStatus
	ObservedMaxLux float64
	ResidentAgreed bool
	Notes          string
	Version        int64
	CreatedAt      time.Time
	CompletedAt    *time.Time
}

func NewInspection(id, caseID, planID, inspectorID ID, now time.Time) InspectionRound {
	return InspectionRound{ID: id, CaseID: caseID, PlanID: planID, InspectorID: inspectorID, Status: InspectionOpen, Version: 1, CreatedAt: now.UTC()}
}

func (v *InspectionRound) Complete(threshold, observed float64, residentAgreed bool, notes string, now time.Time) error {
	if v.Status != InspectionOpen {
		return TransitionError{Entity: "inspection_round", From: string(v.Status), To: "completed"}
	}
	if math.IsNaN(observed) || math.IsInf(observed, 0) || observed < 0 || observed > 200000 {
		return FieldError{Field: "observed_max_lux", Message: "is outside the supported range"}
	}
	notes = strings.TrimSpace(notes)
	if len(notes) > 4000 {
		return FieldError{Field: "notes", Message: "must not exceed 4000 characters"}
	}
	v.ObservedMaxLux = observed
	v.ResidentAgreed = residentAgreed
	v.Notes = notes
	if observed <= threshold && residentAgreed {
		v.Status = InspectionPassed
	} else {
		v.Status = InspectionFailed
	}
	v.Version++
	completed := now.UTC()
	v.CompletedAt = &completed
	return nil
}

func (v InspectionRound) Clone() InspectionRound {
	copyRound := v
	if v.CompletedAt != nil {
		completed := *v.CompletedAt
		copyRound.CompletedAt = &completed
	}
	return copyRound
}
