package service_test

import (
	"context"
	"testing"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func TestCancellingDraftSurveyReturnsCaseToTriage(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey, err := f.service.StartForestSurvey(context.Background(), f.inspector, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.CancelForestSurvey(context.Background(), f.inspector, survey.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := f.service.GetCase(context.Background(), f.admin, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != domain.CaseTriaged {
		t.Fatalf("cancelled draft survey left case in %s", loaded.Status)
	}
}
