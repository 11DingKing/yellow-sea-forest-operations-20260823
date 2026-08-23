package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

func TestForestParcelCannotOpenWithOpenPatrolIssue(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey := f.publishForestSurvey(t, item)
	plan, err := f.service.ProposePlan(context.Background(), f.operator, service.ProposePlanInput{
		CaseID: item.ID, ForestSurveyID: survey.ID,
		Description:  "Complete the remaining patrol remediation before opening.",
		CutoffMinute: 22*60 + 30, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{{
			ForestAssetID: f.forest_assets[0].ID,
			Type:          domain.ActionInstallShield, TargetValue: "installed",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AcceptPlan(context.Background(), f.operator, plan.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ApprovePlan(context.Background(), f.officer, plan.Plan.ID); err != nil {
		t.Fatal(err)
	}

	_, err = f.service.StartInspection(context.Background(), f.inspector, item.ID)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("parcel with an unresolved patrol action was opened: %v", err)
	}
	stored, loadErr := f.service.GetCase(context.Background(), f.admin, item.ID)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if stored.Status != domain.CaseExecuting {
		t.Fatalf("rejected opening changed case status to %s", stored.Status)
	}
}
