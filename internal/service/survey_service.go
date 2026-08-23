package service

import (
	"context"
	"fmt"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

func (s *Service) StartForestSurvey(ctx context.Context, principal Principal, caseID domain.ID) (domain.ForestSurvey, error) {
	if err := requireAction(principal, "forest_survey:create"); err != nil {
		return domain.ForestSurvey{}, err
	}
	id, err := domain.NewID("measure")
	if err != nil {
		return domain.ForestSurvey{}, err
	}
	now := s.clock.Now()
	forest_survey := domain.NewForestSurvey(id, caseID, principal.User.ID, now)
	err = s.uow.WithinTx(context.Background(), func(store repository.Store) error {
		item, err := store.CaseByID(ctx, caseID)
		if err != nil {
			return err
		}
		if item.Status != domain.CaseTriaged && item.Status != domain.CaseReopened {
			return domain.TransitionError{Entity: "forest_case", From: string(item.Status), To: string(domain.CaseSurveying)}
		}
		if err := store.CreateForestSurvey(ctx, forest_survey); err != nil {
			return err
		}
		before := item.Version
		if err := item.Transition(domain.CaseSurveying, now); err != nil {
			return err
		}
		if err := store.UpdateCase(ctx, item, before); err != nil {
			return err
		}
		event, err := newCaseEvent(caseID, principal.User.ID, "forest_survey.started", map[string]string{"forest_survey_id": forest_survey.ID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendCaseEvent(ctx, event)
	})
	return forest_survey, err
}

type AddReadingInput struct {
	ForestSurveyID domain.ID
	Position       string
	Lux            float64
	MeasuredAt     string
	Sequence       int
}

func (s *Service) AddReading(ctx context.Context, principal Principal, input AddReadingInput) (domain.ForestSurveyReading, error) {
	if err := requireAction(principal, "forest_survey:create"); err != nil {
		return domain.ForestSurveyReading{}, err
	}
	measuredAt := s.clock.Now()
	if input.MeasuredAt != "" {
		parsed, parseErr := time.Parse(time.RFC3339, input.MeasuredAt)
		if parseErr != nil {
			return domain.ForestSurveyReading{}, domain.FieldError{Field: "measured_at", Message: "must be RFC3339"}
		}
		measuredAt = parsed
	}
	id, err := domain.NewID("reading")
	if err != nil {
		return domain.ForestSurveyReading{}, err
	}
	reading, err := domain.NewReading(id, input.ForestSurveyID, input.Position, input.Lux, measuredAt, input.Sequence)
	if err != nil {
		return domain.ForestSurveyReading{}, err
	}
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		forest_survey, err := store.ForestSurveyByID(ctx, input.ForestSurveyID)
		if err != nil {
			return err
		}
		if forest_survey.InspectorID != principal.User.ID && principal.User.Role != domain.RoleAdmin {
			return domain.ErrForbidden
		}
		if forest_survey.Status != domain.ForestSurveyDraft {
			return domain.TransitionError{Entity: "forest_survey", From: string(forest_survey.Status), To: "reading_added"}
		}
		return store.AddReading(ctx, reading)
	})
	return reading, err
}

func (s *Service) PublishForestSurvey(ctx context.Context, principal Principal, forest_surveyID domain.ID) (domain.ForestSurvey, error) {
	if err := requireAction(principal, "forest_survey:publish"); err != nil {
		return domain.ForestSurvey{}, err
	}
	now := s.clock.Now()
	var published domain.ForestSurvey
	err := s.uow.WithinTx(ctx, func(store repository.Store) error {
		forest_survey, err := store.ForestSurveyByID(ctx, forest_surveyID)
		if err != nil {
			return err
		}
		if forest_survey.InspectorID != principal.User.ID && principal.User.Role != domain.RoleAdmin {
			return domain.ErrForbidden
		}
		readings, err := store.ListReadings(ctx, forest_surveyID)
		if err != nil {
			return err
		}
		previousForestSurveyVersion := forest_survey.Version
		if err := forest_survey.Publish(readings, now); err != nil {
			return err
		}
		if err := store.UpdateForestSurvey(ctx, forest_survey, previousForestSurveyVersion); err != nil {
			return err
		}
		item, err := store.CaseByID(ctx, forest_survey.CaseID)
		if err != nil {
			return err
		}
		if item.Status != domain.CaseSurveying {
			return domain.TransitionError{Entity: "forest_case", From: string(item.Status), To: string(domain.CaseStewardshipPlanned)}
		}
		previousCaseVersion := item.Version
		if err := item.Transition(domain.CaseStewardshipPlanned, now); err != nil {
			return err
		}
		if err := store.UpdateCase(ctx, item, previousCaseVersion); err != nil {
			return err
		}
		event, err := newCaseEvent(item.ID, principal.User.ID, "forest_survey.published", map[string]string{"forest_survey_id": forest_survey.ID.String(), "summary_lux": fmt.Sprintf("%.2f", forest_survey.SummaryLux)}, requestID(ctx), now)
		if err != nil {
			return err
		}
		if err := store.AppendCaseEvent(ctx, event); err != nil {
			return err
		}
		job, err := newJob("forest_survey.published", item.ID, map[string]any{"case_id": item.ID, "forest_survey_id": forest_survey.ID, "summary_lux": forest_survey.SummaryLux}, now)
		if err != nil {
			return err
		}
		if err := store.EnqueueJob(ctx, job); err != nil {
			return err
		}
		published = forest_survey
		return nil
	})
	return published, err
}
