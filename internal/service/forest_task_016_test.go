package service_test

import (
	"context"
	"errors"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"testing"
)

func TestPlanCreationRollsBackWhenActionBatchFails(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey := f.publishForestSurvey(t, item)
	in := service.ProposePlanInput{CaseID: item.ID, ForestSurveyID: survey.ID, Description: "duplicate remediation actions", CutoffMinute: 1200, ExpectedMaxLux: 8, Actions: []service.ProposedAction{{ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionInstallShield, TargetValue: "installed"}, {ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionInstallShield, TargetValue: "installed"}}}
	if _, e := f.service.ProposePlan(context.Background(), f.operator, in); e == nil {
		t.Fatal("duplicate plan unexpectedly succeeded")
	}
	if plan, e := f.database.Store().PlanForCase(context.Background(), item.ID); !errors.Is(e, domain.ErrNotFound) {
		t.Fatalf("orphan plan persisted: %#v err=%v", plan, e)
	}
}
