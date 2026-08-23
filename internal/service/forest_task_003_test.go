package service_test

import (
	"context"
	"testing"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

func TestReleasedPatrolLeaseCanBeClaimedAgain(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey := f.publishForestSurvey(t, item)
	plan, err := f.service.ProposePlan(context.Background(), f.operator, service.ProposePlanInput{
		CaseID: item.ID, ForestSurveyID: survey.ID, Description: "lease release test",
		CutoffMinute: 1350, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{{ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionInstallShield, TargetValue: "installed"}},
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
	claimed, err := f.service.ClaimForestAssetAction(context.Background(), f.operator, plan.Actions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.ReleaseForestAssetAction(context.Background(), f.operator, claimed.ID, claimed.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ClaimForestAssetAction(context.Background(), f.admin, claimed.ID); err != nil {
		t.Fatalf("released patrol lease was not claimable: %v", err)
	}
}
