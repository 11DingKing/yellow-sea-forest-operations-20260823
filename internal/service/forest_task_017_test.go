package service_test

import (
	"context"
	"errors"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"testing"
)

func TestCancelledPublishDoesNotAdvanceWorkflow(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey, e := f.service.StartForestSurvey(context.Background(), f.inspector, item.ID)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = f.service.PublishForestSurvey(ctx, f.inspector, survey.ID)
	if !errors.Is(e, context.Canceled) {
		t.Fatalf("cancelled publish error=%v", e)
	}
	loaded, e := f.database.Store().ForestSurveyByID(context.Background(), survey.ID)
	if e != nil {
		t.Fatal(e)
	}
	if loaded.Status != domain.ForestSurveyDraft {
		t.Fatalf("cancelled publish persisted")
	}
}
