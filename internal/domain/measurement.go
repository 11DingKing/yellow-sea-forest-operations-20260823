package domain

import (
	"math"
	"sort"
	"strings"
	"time"
)

type ForestSurveyStatus string

const (
	ForestSurveyDraft     ForestSurveyStatus = "draft"
	ForestSurveyPublished ForestSurveyStatus = "published"
	ForestSurveyCancelled ForestSurveyStatus = "cancelled"
)

type ForestSurvey struct {
	ID          ID
	CaseID      ID
	InspectorID ID
	Status      ForestSurveyStatus
	StartedAt   time.Time
	PublishedAt *time.Time
	SummaryLux  float64
	Version     int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ForestSurveyReading struct {
	ID             ID
	ForestSurveyID ID
	Position       string
	Lux            float64
	MeasuredAt     time.Time
	Sequence       int
}

func NewForestSurvey(id, caseID, inspectorID ID, now time.Time) ForestSurvey {
	now = now.UTC()
	return ForestSurvey{ID: id, CaseID: caseID, InspectorID: inspectorID, Status: ForestSurveyDraft, StartedAt: now, Version: 1, CreatedAt: now, UpdatedAt: now}
}

func NewReading(id, forest_surveyID ID, position string, lux float64, measuredAt time.Time, sequence int) (ForestSurveyReading, error) {
	position = strings.TrimSpace(position)
	if len(position) < 2 || len(position) > 160 {
		return ForestSurveyReading{}, FieldError{Field: "position", Message: "must contain 2 to 160 characters"}
	}
	if math.IsNaN(lux) || math.IsInf(lux, 0) || lux < 0 || lux > 200000 {
		return ForestSurveyReading{}, FieldError{Field: "lux", Message: "is outside the supported range"}
	}
	if sequence <= 0 {
		return ForestSurveyReading{}, FieldError{Field: "sequence", Message: "must be positive"}
	}
	return ForestSurveyReading{ID: id, ForestSurveyID: forest_surveyID, Position: position, Lux: lux, MeasuredAt: measuredAt.UTC(), Sequence: sequence}, nil
}

func (m *ForestSurvey) Publish(readings []ForestSurveyReading, now time.Time) error {
	if m.Status != ForestSurveyDraft {
		return TransitionError{Entity: "forest_survey", From: string(m.Status), To: string(ForestSurveyPublished)}
	}
	if len(readings) < 3 {
		return FieldError{Field: "readings", Message: "at least three readings are required"}
	}
	seen := make(map[int]struct{}, len(readings))
	values := make([]float64, 0, len(readings))
	for _, reading := range readings {
		if reading.ForestSurveyID != m.ID {
			return FieldError{Field: "readings", Message: "contains another forest_survey"}
		}
		if _, exists := seen[reading.Sequence]; exists {
			return ConflictError{Resource: "forest_survey_reading", Reason: "duplicate sequence"}
		}
		seen[reading.Sequence] = struct{}{}
		values = append(values, reading.Lux)
	}
	sort.Float64s(values)
	trim := 0
	if len(values) >= 5 {
		trim = 1
	}
	var total float64
	for _, value := range values[trim : len(values)-trim] {
		total += value
	}
	m.SummaryLux = total / float64(len(values)-2*trim)
	m.Status = ForestSurveyPublished
	published := now.UTC()
	m.PublishedAt = &published
	m.Version++
	m.UpdatedAt = published
	return nil
}

func CloneReadings(readings []ForestSurveyReading) []ForestSurveyReading {
	return append([]ForestSurveyReading(nil), readings...)
}

func NewReadingBatch(values []ForestSurveyReading) []ForestSurveyReading {
	return CloneReadings(values)
}
