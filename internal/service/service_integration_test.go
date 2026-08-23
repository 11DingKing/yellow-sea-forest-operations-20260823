package service_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/clock"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/storage/sqlite"
)

var serviceNow = time.Date(2026, 8, 20, 20, 0, 0, 0, time.FixedZone("CST", 8*60*60))

type serviceForestAsset struct {
	database      *sqlite.Database
	clock         *clock.Manual
	service       *service.Service
	admin         service.Principal
	officer       service.Principal
	inspector     service.Principal
	operator      service.Principal
	liaison       service.Principal
	forest_site   domain.ForestSite
	zone          domain.ForestParcel
	forest_assets []domain.ForestAsset
}

func newServiceForestAsset(t *testing.T) *serviceForestAsset {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service.db")
	database, err := sqlite.Open(context.Background(), "file:"+filepath.ToSlash(path)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	manual := clock.NewManual(serviceNow)
	svc := service.New(database, manual, 12*time.Hour)
	if _, err := svc.EnsureBootstrapAdmin(context.Background(), "admin@example.com", "administrator-password"); err != nil {
		t.Fatal(err)
	}
	forest_asset := &serviceForestAsset{database: database, clock: manual, service: svc}
	forest_asset.admin = forest_asset.login(t, "admin@example.com", "administrator-password")
	forest_asset.officer = forest_asset.registerAndLogin(t, "officer@example.com", "Night Duty Officer", domain.RoleOfficer)
	forest_asset.inspector = forest_asset.registerAndLogin(t, "inspector@example.com", "Field Inspector", domain.RoleInspector)
	forest_asset.operator = forest_asset.registerAndLogin(t, "operator@example.com", "ForestSite Operator", domain.RoleOperator)
	forest_asset.liaison = forest_asset.registerAndLogin(t, "liaison@example.com", "Resident Liaison", domain.RoleResidentLiaison)
	forest_asset.forest_site, err = svc.CreateForestSite(context.Background(), forest_asset.admin, service.CreateForestSiteInput{
		Name: "Cuihu Sports Park", OperatorID: forest_asset.operator.User.ID, Address: "Luohu District, Shenzhen",
		Timezone: "Asia/Shanghai", CutoffMinute: 22*60 + 30, Latitude: 22.57, Longitude: 114.14,
	})
	if err != nil {
		t.Fatal(err)
	}
	forest_asset.zone, err = svc.CreateForestParcel(context.Background(), forest_asset.admin, service.CreateZoneInput{
		Name: "Lakeview Phase One", Address: "Taoyuan Road, Luohu", ContactUserID: forest_asset.liaison.User.ID,
		WindowCount: 820, SensitivityLux: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 3; index++ {
		created, err := svc.CreateForestAsset(context.Background(), forest_asset.admin, service.CreateForestAssetInput{
			ForestSiteID: forest_asset.forest_site.ID, Label: fmt.Sprintf("Residential Row %d", index), RowNumber: index,
			Orientation: "toward residential windows", Angle: -20,
		})
		if err != nil {
			t.Fatal(err)
		}
		forest_asset.forest_assets = append(forest_asset.forest_assets, created)
	}
	return forest_asset
}

func (f *serviceForestAsset) login(t *testing.T, email, password string) service.Principal {
	t.Helper()
	result, err := f.service.Login(context.Background(), service.LoginInput{Email: email, Password: password, UserAgent: "service-test", IP: "127.0.0.1"})
	if err != nil {
		t.Fatalf("Login(%s) error = %v", email, err)
	}
	principal, err := f.service.Authenticate(context.Background(), result.Token)
	if err != nil {
		t.Fatalf("Authenticate(%s) error = %v", email, err)
	}
	return principal
}

func (f *serviceForestAsset) registerAndLogin(t *testing.T, email, displayName string, role domain.Role) service.Principal {
	t.Helper()
	password := "role-account-password"
	if _, err := f.service.RegisterUser(context.Background(), f.admin, service.RegisterUserInput{Email: email, DisplayName: displayName, Password: password, Role: role}); err != nil {
		t.Fatalf("RegisterUser(%s) error = %v", role, err)
	}
	return f.login(t, email, password)
}

func (f *serviceForestAsset) submitAndAssign(t *testing.T) domain.ForestCase {
	t.Helper()
	result, err := f.service.SubmitCase(context.Background(), f.liaison, service.SubmitCaseInput{
		ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "Night football lighting reaches homes",
		Description: "Two strong white lighting towers illuminate balconies and bedroom windows every night.", Priority: 4,
		IdempotencyKey: "complaint-request-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	assigned, err := f.service.AssignCase(context.Background(), f.officer, service.AssignCaseInput{CaseID: result.Case.ID, ExpectedVersion: result.Case.Version, OwnerID: f.officer.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	return assigned
}

func (f *serviceForestAsset) publishForestSurvey(t *testing.T, item domain.ForestCase) domain.ForestSurvey {
	t.Helper()
	forest_survey, err := f.service.StartForestSurvey(context.Background(), f.inspector, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index, lux := range []float64{24, 28, 32} {
		if _, err := f.service.AddReading(context.Background(), f.inspector, service.AddReadingInput{ForestSurveyID: forest_survey.ID, Position: fmt.Sprintf("window-%d", index+1), Lux: lux, Sequence: index + 1}); err != nil {
			t.Fatal(err)
		}
	}
	published, err := f.service.PublishForestSurvey(context.Background(), f.inspector, forest_survey.ID)
	if err != nil {
		t.Fatal(err)
	}
	return published
}

func TestCompleteComplaintStewardshipWorkflow(t *testing.T) {
	f := newServiceForestAsset(t)
	submitted, err := f.service.SubmitCase(context.Background(), f.liaison, service.SubmitCaseInput{
		ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "Night football lighting reaches homes",
		Description: "Two strong white lighting towers illuminate balconies and bedroom windows every night.", Priority: 4,
		IdempotencyKey: "complaint-request-001",
	})
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := f.service.SubmitCase(context.Background(), f.liaison, service.SubmitCaseInput{
		ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "Night football lighting reaches homes",
		Description: "Two strong white lighting towers illuminate balconies and bedroom windows every night.", Priority: 4,
		IdempotencyKey: "complaint-request-001",
	})
	if err != nil || !replayed.Replayed || replayed.Case.ID != submitted.Case.ID {
		t.Fatalf("idempotent replay = %#v, %v", replayed, err)
	}
	assigned, err := f.service.AssignCase(context.Background(), f.officer, service.AssignCaseInput{CaseID: submitted.Case.ID, ExpectedVersion: submitted.Case.Version, OwnerID: f.officer.User.ID})
	if err != nil || assigned.Status != domain.CaseTriaged {
		t.Fatalf("AssignCase() = %#v, %v", assigned, err)
	}
	forest_survey, err := f.service.StartForestSurvey(context.Background(), f.inspector, assigned.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index, lux := range []float64{26, 32, 29, 120, 24} {
		reading, err := f.service.AddReading(context.Background(), f.inspector, service.AddReadingInput{
			ForestSurveyID: forest_survey.ID, Position: fmt.Sprintf("building-window-%d", index+1), Lux: lux,
			MeasuredAt: serviceNow.Add(time.Duration(index) * time.Minute).Format(time.RFC3339), Sequence: index + 1,
		})
		if err != nil || reading.Sequence != index+1 {
			t.Fatalf("AddReading(%d) = %#v, %v", index, reading, err)
		}
	}
	published, err := f.service.PublishForestSurvey(context.Background(), f.inspector, forest_survey.ID)
	if err != nil || published.Status != domain.ForestSurveyPublished || published.SummaryLux != 29 {
		t.Fatalf("PublishForestSurvey() = %#v, %v", published, err)
	}
	planResult, err := f.service.ProposePlan(context.Background(), f.operator, service.ProposePlanInput{
		CaseID: assigned.ID, ForestSurveyID: published.ID, Description: "Install shields, lower residential-facing lamps, and disable the second row.",
		CutoffMinute: 22*60 + 30, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{
			{ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionInstallShield, TargetValue: "installed"},
			{ForestAssetID: f.forest_assets[1].ID, Type: domain.ActionAdjustAngle, TargetValue: "-55"},
			{ForestAssetID: f.forest_assets[2].ID, Type: domain.ActionDisableRow, TargetValue: "disabled"},
		},
	})
	if err != nil || len(planResult.Actions) != 3 {
		t.Fatalf("ProposePlan() = %#v, %v", planResult, err)
	}
	accepted, err := f.service.AcceptPlan(context.Background(), f.operator, planResult.Plan.ID)
	if err != nil || accepted.Status != domain.PlanOperatorAccepted {
		t.Fatalf("AcceptPlan() = %#v, %v", accepted, err)
	}
	approved, err := f.service.ApprovePlan(context.Background(), f.officer, planResult.Plan.ID)
	if err != nil || approved.Status != domain.PlanApproved {
		t.Fatalf("ApprovePlan() = %#v, %v", approved, err)
	}
	for _, action := range planResult.Actions {
		claimed, err := f.service.ClaimForestAssetAction(context.Background(), f.operator, action.ID)
		if err != nil {
			t.Fatalf("ClaimForestAssetAction(%s) error = %v", action.ID, err)
		}
		if err := f.service.CompleteForestAssetAction(context.Background(), f.operator, claimed.ID, claimed.Version); err != nil {
			t.Fatalf("CompleteForestAssetAction(%s) error = %v", claimed.ID, err)
		}
	}
	forest_assets, err := f.service.ListForestAssets(context.Background(), f.officer, f.forest_site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !forest_assets[0].Shielded || forest_assets[1].AngleDegrees != -55 || forest_assets[2].Enabled {
		t.Fatalf("mitigated forest_assets = %#v", forest_assets)
	}
	round, err := f.service.StartInspection(context.Background(), f.inspector, assigned.ID)
	if err != nil || round.Status != domain.InspectionOpen {
		t.Fatalf("StartInspection() = %#v, %v", round, err)
	}
	completed, err := f.service.CompleteInspection(context.Background(), f.officer, service.CompleteInspectionInput{InspectionID: round.ID, ObservedMaxLux: 7.5, ResidentAgreed: true, Notes: "Professional night survey and resident follow-up passed."})
	if err != nil || completed.Status != domain.InspectionPassed {
		t.Fatalf("CompleteInspection() = %#v, %v", completed, err)
	}
	resolved, err := f.service.GetCase(context.Background(), f.liaison, assigned.ID)
	if err != nil || resolved.Status != domain.CaseResolved || resolved.ReopenUntil == nil {
		t.Fatalf("resolved case = %#v, %v", resolved, err)
	}
	reopened, err := f.service.ReopenCase(context.Background(), f.liaison, service.ReopenCaseInput{CaseID: assigned.ID, IdempotencyKey: "reopen-request-001", Reason: "Strong spill light remains visible from two bedroom windows."})
	if err != nil || reopened.Status != domain.CaseReopened {
		t.Fatalf("ReopenCase() = %#v, %v", reopened, err)
	}
	replayedReopen, err := f.service.ReopenCase(context.Background(), f.liaison, service.ReopenCaseInput{CaseID: assigned.ID, IdempotencyKey: "reopen-request-001", Reason: "Strong spill light remains visible from two bedroom windows."})
	if err != nil || replayedReopen.ID != reopened.ID || replayedReopen.Version != reopened.Version {
		t.Fatalf("replayed reopen = %#v, %v", replayedReopen, err)
	}
}

func TestSubmitCaseRejectsKeyReuseForDifferentRequest(t *testing.T) {
	f := newServiceForestAsset(t)
	input := service.SubmitCaseInput{ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "Original glare report", Description: "Strong light reaches the residential building every evening.", Priority: 4, IdempotencyKey: "same-request-key"}
	if _, err := f.service.SubmitCase(context.Background(), f.liaison, input); err != nil {
		t.Fatal(err)
	}
	input.Description = "A materially different complaint is submitted with the reused key."
	if _, err := f.service.SubmitCase(context.Background(), f.liaison, input); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("key reuse error = %v", err)
	}
}

func TestReopenRejectsKeyReuseForDifferentReason(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	loaded, err := f.database.Store().CaseByID(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Status = domain.CaseResolved
	resolvedAt := f.clock.Now()
	reopenUntil := resolvedAt.Add(14 * 24 * time.Hour)
	loaded.ResolvedAt, loaded.ReopenUntil = &resolvedAt, &reopenUntil
	loaded.Version++
	if err := f.database.Store().UpdateCase(context.Background(), loaded, loaded.Version-1); err != nil {
		t.Fatal(err)
	}
	input := service.ReopenCaseInput{CaseID: item.ID, IdempotencyKey: "reopen-same-key", Reason: "The shield still leaves direct glare at the east bedroom."}
	if _, err := f.service.ReopenCase(context.Background(), f.liaison, input); err != nil {
		t.Fatal(err)
	}
	input.Reason = "The cutoff schedule did not execute on the second field row."
	if _, err := f.service.ReopenCase(context.Background(), f.liaison, input); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reopen key reuse error = %v", err)
	}
}

func TestAuthorizationStopsCrossRoleOperations(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	if _, err := f.service.StartForestSurvey(context.Background(), f.operator, item.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("operator StartForestSurvey() error = %v", err)
	}
	if _, err := f.service.AssignCase(context.Background(), f.liaison, service.AssignCaseInput{CaseID: item.ID, ExpectedVersion: item.Version, OwnerID: f.officer.User.ID}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("liaison AssignCase() error = %v", err)
	}
	if _, err := f.service.CreateForestAsset(context.Background(), f.inspector, service.CreateForestAssetInput{ForestSiteID: f.forest_site.ID, Label: "Unauthorized Lamp", RowNumber: 9, Orientation: "north", Angle: -20}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("inspector CreateForestAsset() error = %v", err)
	}
	forest_assets, err := f.service.ListForestAssets(context.Background(), f.admin, f.forest_site.ID)
	if err != nil || len(forest_assets) != 3 {
		t.Fatalf("unauthorized operation changed forest_assets: %#v, %v", forest_assets, err)
	}
}

func TestPlanValidationRollsBackAllEntities(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	forest_survey, err := f.service.StartForestSurvey(context.Background(), f.inspector, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 3; index++ {
		if _, err := f.service.AddReading(context.Background(), f.inspector, service.AddReadingInput{ForestSurveyID: forest_survey.ID, Position: fmt.Sprintf("window-%d", index), Lux: float64(20 + index), Sequence: index}); err != nil {
			t.Fatal(err)
		}
	}
	published, err := f.service.PublishForestSurvey(context.Background(), f.inspector, forest_survey.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.ProposePlan(context.Background(), f.operator, service.ProposePlanInput{
		CaseID: item.ID, ForestSurveyID: published.ID, Description: "Attempt a plan containing a forest_asset from another forest_site.", CutoffMinute: 1350, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{{ForestAssetID: "forest_asset_missing", Type: domain.ActionInstallShield, TargetValue: "installed"}},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("ProposePlan() error = %v", err)
	}
	if _, err := f.database.Store().PlanForCase(context.Background(), item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rolled-back plan lookup error = %v", err)
	}
}

func TestServicePropagatesCancelledContext(t *testing.T) {
	f := newServiceForestAsset(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f.service.SubmitCase(ctx, f.liaison, service.SubmitCaseInput{ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "Cancelled complaint", Description: "This request must not persist after its context is cancelled.", Priority: 3, IdempotencyKey: "cancelled-request"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SubmitCase(cancelled) error = %v", err)
	}
	page, err := f.database.Store().ListCases(context.Background(), domain.CaseFilter{}, domain.PageRequest{Page: 1, PageSize: 10})
	if err != nil || page.Total != 0 {
		t.Fatalf("cancelled request persisted cases: %#v, %v", page, err)
	}
}

func TestLogoutRevokesOnlyCurrentSession(t *testing.T) {
	f := newServiceForestAsset(t)
	first, err := f.service.Login(context.Background(), service.LoginInput{Email: f.officer.User.Email, Password: "role-account-password"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Login(context.Background(), service.LoginInput{Email: f.officer.User.Email, Password: "role-account-password"})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := f.service.Authenticate(context.Background(), first.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.Logout(context.Background(), principal); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Authenticate(context.Background(), first.Token); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("revoked token error = %v", err)
	}
	if _, err := f.service.Authenticate(context.Background(), second.Token); err != nil {
		t.Fatalf("other session was revoked: %v", err)
	}
}

func TestPublishedForestSurveyRejectsLateReadings(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	published := f.publishForestSurvey(t, item)
	_, err := f.service.AddReading(context.Background(), f.inspector, service.AddReadingInput{ForestSurveyID: published.ID, Position: "late-window", Lux: 200, Sequence: 4})
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("late AddReading() error = %v", err)
	}
	readings, err := f.database.Store().ListReadings(context.Background(), published.ID)
	if err != nil || len(readings) != 3 {
		t.Fatalf("late reading persisted: %#v, %v", readings, err)
	}
}

func TestOperatorCannotProposeOrClaimWorkForAnotherForestSite(t *testing.T) {
	f := newServiceForestAsset(t)
	other := f.registerAndLogin(t, "other-operator@example.com", "Other ForestSite Operator", domain.RoleOperator)
	item := f.submitAndAssign(t)
	published := f.publishForestSurvey(t, item)
	input := service.ProposePlanInput{
		CaseID: item.ID, ForestSurveyID: published.ID, Description: "Install shielding on the residential-facing lighting tower.", CutoffMinute: 1350, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{{ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionInstallShield, TargetValue: "installed"}},
	}
	if _, err := f.service.ProposePlan(context.Background(), other, input); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other operator ProposePlan() error = %v", err)
	}
	if _, err := f.database.Store().PlanForCase(context.Background(), item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("unauthorized plan persisted: %v", err)
	}
	result, err := f.service.ProposePlan(context.Background(), f.operator, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ClaimForestAssetAction(context.Background(), f.operator, result.Actions[0].ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("claim before approval error = %v", err)
	}
	if _, err := f.service.AcceptPlan(context.Background(), f.operator, result.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ApprovePlan(context.Background(), f.officer, result.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ClaimForestAssetAction(context.Background(), other, result.Actions[0].ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("other operator ClaimForestAssetAction() error = %v", err)
	}
	action, err := f.database.Store().ActionByID(context.Background(), result.Actions[0].ID)
	if err != nil || action.Status != domain.ActionPending || action.WorkerID != nil {
		t.Fatalf("unauthorized claim changed action: %#v, %v", action, err)
	}
}

func TestScheduleCutoffActionUpdatesForestSiteAtomically(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	published := f.publishForestSurvey(t, item)
	result, err := f.service.ProposePlan(context.Background(), f.operator, service.ProposePlanInput{
		CaseID: item.ID, ForestSurveyID: published.ID, Description: "Move the automatic field shutdown earlier to protect nearby homes.", CutoffMinute: 22 * 60, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{{ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionScheduleCutoff, TargetValue: "1320"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.AcceptPlan(context.Background(), f.operator, result.Plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ApprovePlan(context.Background(), f.officer, result.Plan.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err := f.service.ClaimForestAssetAction(context.Background(), f.operator, result.Actions[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.CompleteForestAssetAction(context.Background(), f.operator, claimed.ID, claimed.Version); err != nil {
		t.Fatal(err)
	}
	forest_site, err := f.database.Store().ForestSiteByID(context.Background(), f.forest_site.ID)
	if err != nil || forest_site.CutoffMinute != 22*60 || forest_site.Version != f.forest_site.Version+1 {
		t.Fatalf("updated forest_site = %#v, %v", forest_site, err)
	}
	action, err := f.database.Store().ActionByID(context.Background(), claimed.ID)
	if err != nil || action.Status != domain.ActionCompleted {
		t.Fatalf("completed schedule action = %#v, %v", action, err)
	}
}

func TestScheduleCutoffMustMatchPlan(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	published := f.publishForestSurvey(t, item)
	_, err := f.service.ProposePlan(context.Background(), f.operator, service.ProposePlanInput{
		CaseID: item.ID, ForestSurveyID: published.ID, Description: "Configure an earlier automatic cutoff for all field lighting.", CutoffMinute: 1320, ExpectedMaxLux: 8,
		Actions: []service.ProposedAction{{ForestAssetID: f.forest_assets[0].ID, Type: domain.ActionScheduleCutoff, TargetValue: "1350"}},
	})
	if !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("mismatched cutoff error = %v", err)
	}
}
