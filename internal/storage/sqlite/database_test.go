package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

var storageNow = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

type forest_assetSet struct {
	admin         domain.User
	officer       domain.User
	inspector     domain.User
	operator      domain.User
	liaison       domain.User
	forest_site   domain.ForestSite
	zone          domain.ForestParcel
	forest_assets []domain.ForestAsset
}

func openTestDatabase(t *testing.T) (*Database, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "forest-operations.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	database, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, dsn
}

func makeUser(t *testing.T, id domain.ID, role domain.Role) domain.User {
	t.Helper()
	user, err := domain.NewUser(id, id.String()+"@example.com", "User "+id.String(), []byte("not-used-in-storage-tests"), role, storageNow)
	if err != nil {
		t.Fatalf("NewUser(%s) error = %v", id, err)
	}
	return user
}

func seedForestAssetSet(t *testing.T, database *Database) forest_assetSet {
	t.Helper()
	set := forest_assetSet{
		admin: makeUser(t, "admin_001", domain.RoleAdmin), officer: makeUser(t, "officer_001", domain.RoleOfficer),
		inspector: makeUser(t, "inspector_001", domain.RoleInspector), operator: makeUser(t, "operator_001", domain.RoleOperator),
		liaison: makeUser(t, "liaison_001", domain.RoleResidentLiaison),
	}
	store := database.Store()
	for _, user := range []domain.User{set.admin, set.officer, set.inspector, set.operator, set.liaison} {
		if err := store.CreateUser(context.Background(), user); err != nil {
			t.Fatalf("CreateUser(%s) error = %v", user.ID, err)
		}
	}
	var err error
	set.forest_site, err = domain.NewForestSite("forest_site_001", "Cuihu Sports Park", set.operator.ID, "Luohu District", "Asia/Shanghai", 22*60+30, storageNow)
	if err != nil {
		t.Fatal(err)
	}
	set.forest_site.Latitude, set.forest_site.Longitude = 22.57, 114.14
	set.zone, err = domain.NewForestParcel("zone_001", "Lakeview Phase One", "Taoyuan Road", set.liaison.ID, 820, 8, storageNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateForestSite(context.Background(), set.forest_site); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateZone(context.Background(), set.zone); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 3; index++ {
		forest_asset, forest_assetErr := domain.NewForestAsset(domain.ID(fmt.Sprintf("forest_asset_00%d", index)), set.forest_site.ID, fmt.Sprintf("Row %d Lamp", index), index, "toward residential windows", -20, storageNow)
		if forest_assetErr != nil {
			t.Fatal(forest_assetErr)
		}
		if err := store.CreateForestAsset(context.Background(), forest_asset); err != nil {
			t.Fatal(err)
		}
		set.forest_assets = append(set.forest_assets, forest_asset)
	}
	return set
}

func makeCase(t *testing.T, id domain.ID, set forest_assetSet, submitted time.Time, priority int) domain.ForestCase {
	t.Helper()
	item, err := domain.NewForestCase(id, set.forest_site.ID, set.zone.ID, set.liaison.ID, "Strong field lighting", "Strong white light reaches homes beside the football field.", priority, submitted)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestOpenMigratesSchemaAndPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restart.db")
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	first, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if version, err := first.SchemaVersion(context.Background()); err != nil || version != currentSchemaVersion {
		t.Fatalf("SchemaVersion() = %d, %v", version, err)
	}
	user := makeUser(t, "admin_restart", domain.RoleAdmin)
	if err := first.Store().CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	loaded, err := second.Store().UserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("UserByID() after restart error = %v", err)
	}
	if loaded.Email != user.Email || loaded.Role != domain.RoleAdmin {
		t.Fatalf("loaded user = %#v", loaded)
	}
}

func TestWithinTxCommitsAllWrites(t *testing.T) {
	database, _ := openTestDatabase(t)
	user := makeUser(t, "admin_commit", domain.RoleAdmin)
	audit := domain.AuditEvent{ID: "audit_commit", ActorID: user.ID, Action: "bootstrap", ObjectType: "user", ObjectID: user.ID, Result: "success", RequestID: "request_commit", Metadata: map[string]string{"source": "test"}, CreatedAt: storageNow}
	err := database.WithinTx(context.Background(), func(store repository.Store) error {
		if err := store.CreateUser(context.Background(), user); err != nil {
			return err
		}
		return store.AppendAudit(context.Background(), audit)
	})
	if err != nil {
		t.Fatalf("WithinTx() error = %v", err)
	}
	if _, err := database.Store().UserByID(context.Background(), user.ID); err != nil {
		t.Fatalf("committed user not found: %v", err)
	}
	events, err := database.Store().ListAudit(context.Background(), "user", user.ID, 10)
	if err != nil || len(events) != 1 || events[0].Metadata["source"] != "test" {
		t.Fatalf("committed audit = %#v, %v", events, err)
	}
}

func TestWithinTxRollsBackAllWrites(t *testing.T) {
	database, _ := openTestDatabase(t)
	user := makeUser(t, "admin_rollback", domain.RoleAdmin)
	sentinel := errors.New("stop transaction")
	err := database.WithinTx(context.Background(), func(store repository.Store) error {
		if err := store.CreateUser(context.Background(), user); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithinTx() error = %v", err)
	}
	if _, err := database.Store().UserByID(context.Background(), user.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("rolled-back user lookup error = %v", err)
	}
}

func TestWithinTxHonorsCancelledContext(t *testing.T) {
	database, _ := openTestDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := database.WithinTx(ctx, func(repository.Store) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("WithinTx(cancelled) = %v, called=%v", err, called)
	}
}

func TestUserSessionLifecycle(t *testing.T) {
	database, _ := openTestDatabase(t)
	set := seedForestAssetSet(t, database)
	expires := storageNow.Add(time.Hour)
	session := domain.Session{ID: "session_001", UserID: set.admin.ID, TokenHash: []byte("unique-token-hash"), CreatedAt: storageNow, ExpiresAt: expires, UserAgent: "integration-test", IP: "127.0.0.1"}
	store := database.Store()
	if err := store.CreateSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.SessionByTokenHash(context.Background(), session.TokenHash)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != session.ID || loaded.UserID != session.UserID || !loaded.ExpiresAt.Equal(expires) {
		t.Fatalf("loaded session = %#v", loaded)
	}
	revokedAt := storageNow.Add(10 * time.Minute)
	if err := store.RevokeSession(context.Background(), session.ID, revokedAt); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.SessionByTokenHash(context.Background(), session.TokenHash)
	if err != nil || loaded.RevokedAt == nil || !loaded.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoked session = %#v, %v", loaded, err)
	}
	expired := domain.Session{ID: "session_expired", UserID: set.admin.ID, TokenHash: []byte("expired-token"), CreatedAt: storageNow.Add(-2 * time.Hour), ExpiresAt: storageNow.Add(-time.Hour)}
	if err := store.CreateSession(context.Background(), expired); err != nil {
		t.Fatal(err)
	}
	removed, err := store.DeleteExpiredSessions(context.Background(), storageNow, 10)
	if err != nil || removed != 2 {
		t.Fatalf("DeleteExpiredSessions() = %d, %v", removed, err)
	}
}

func TestForestSiteZoneAndForestAssetPersistence(t *testing.T) {
	database, _ := openTestDatabase(t)
	set := seedForestAssetSet(t, database)
	store := database.Store()
	forest_site, err := store.ForestSiteByID(context.Background(), set.forest_site.ID)
	if err != nil || forest_site.Name != set.forest_site.Name || forest_site.OperatorID != set.operator.ID {
		t.Fatalf("ForestSiteByID() = %#v, %v", forest_site, err)
	}
	zone, err := store.ZoneByID(context.Background(), set.zone.ID)
	if err != nil || zone.ContactUserID != set.liaison.ID || zone.WindowCount != 820 {
		t.Fatalf("ZoneByID() = %#v, %v", zone, err)
	}
	forest_assets, err := store.ListForestAssets(context.Background(), set.forest_site.ID)
	if err != nil || len(forest_assets) != 3 {
		t.Fatalf("ListForestAssets() = %#v, %v", forest_assets, err)
	}
	if forest_assets[0].RowNumber != 1 || forest_assets[2].RowNumber != 3 {
		t.Fatalf("forest_assets are not ordered: %#v", forest_assets)
	}
	forest_asset, err := store.ForestAssetByID(context.Background(), set.forest_assets[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	before := forest_asset.Version
	forest_asset.Shielded = true
	forest_asset.AngleDegrees = -45
	forest_asset.Version++
	forest_asset.UpdatedAt = storageNow.Add(time.Hour)
	if err := store.UpdateForestAsset(context.Background(), forest_asset, before); err != nil {
		t.Fatal(err)
	}
	updated, err := store.ForestAssetByID(context.Background(), forest_asset.ID)
	if err != nil || !updated.Shielded || updated.AngleDegrees != -45 || updated.Version != before+1 {
		t.Fatalf("updated forest_asset = %#v, %v", updated, err)
	}
	if err := store.UpdateForestAsset(context.Background(), forest_asset, before); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale UpdateForestAsset() error = %v", err)
	}
}

func TestCasePersistenceEventsFilteringAndPagination(t *testing.T) {
	database, _ := openTestDatabase(t)
	set := seedForestAssetSet(t, database)
	store := database.Store()
	for index := 1; index <= 7; index++ {
		item := makeCase(t, domain.ID(fmt.Sprintf("case_%03d", index)), set, storageNow.Add(time.Duration(index)*time.Minute), 1+(index%5))
		if err := store.CreateCase(context.Background(), item); err != nil {
			t.Fatal(err)
		}
		event := domain.CaseEvent{ID: domain.ID(fmt.Sprintf("event_%03d", index)), CaseID: item.ID, ActorID: set.liaison.ID, Type: "case.submitted", Payload: map[string]string{"index": fmt.Sprint(index)}, RequestID: fmt.Sprintf("request_%03d", index), CreatedAt: item.SubmittedAt}
		if err := store.AppendCaseEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	page, err := store.ListCases(context.Background(), domain.CaseFilter{Statuses: []domain.CaseStatus{domain.CaseSubmitted}, Search: "white light"}, domain.PageRequest{Page: 2, PageSize: 3, Sort: "submitted_at", Desc: false})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 7 || page.Page != 2 || page.PageSize != 3 || len(page.Items) != 3 {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].ID != "case_004" || page.Items[2].ID != "case_006" {
		t.Fatalf("page order = %#v", page.Items)
	}
	events, err := store.ListCaseEvents(context.Background(), "case_003")
	if err != nil || len(events) != 1 || events[0].Payload["index"] != "3" {
		t.Fatalf("events = %#v, %v", events, err)
	}
	item, err := store.CaseByID(context.Background(), "case_001")
	if err != nil {
		t.Fatal(err)
	}
	before := item.Version
	if err := item.Assign(set.officer.ID, item.Version, storageNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCase(context.Background(), item, before); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateCase(context.Background(), item, before); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale UpdateCase() error = %v", err)
	}
}

func TestForestSurveyPersistenceAndOpenUniqueness(t *testing.T) {
	database, _ := openTestDatabase(t)
	set := seedForestAssetSet(t, database)
	item := makeCase(t, "case_forest_survey", set, storageNow, 4)
	store := database.Store()
	if err := store.CreateCase(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	forest_survey := domain.NewForestSurvey("forest_survey_001", item.ID, set.inspector.ID, storageNow)
	if err := store.CreateForestSurvey(context.Background(), forest_survey); err != nil {
		t.Fatal(err)
	}
	duplicate := domain.NewForestSurvey("forest_survey_002", item.ID, set.inspector.ID, storageNow)
	if err := store.CreateForestSurvey(context.Background(), duplicate); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second open forest_survey error = %v", err)
	}
	for sequence, lux := range []float64{10, 20, 30, 40, 100} {
		reading, err := domain.NewReading(domain.ID(fmt.Sprintf("reading_%03d", sequence+1)), forest_survey.ID, fmt.Sprintf("window-%d", sequence+1), lux, storageNow.Add(time.Duration(sequence)*time.Minute), sequence+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.AddReading(context.Background(), reading); err != nil {
			t.Fatal(err)
		}
	}
	readings, err := store.ListReadings(context.Background(), forest_survey.ID)
	if err != nil || len(readings) != 5 || readings[0].Sequence != 1 || readings[4].Sequence != 5 {
		t.Fatalf("ListReadings() = %#v, %v", readings, err)
	}
	before := forest_survey.Version
	if err := forest_survey.Publish(readings, storageNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateForestSurvey(context.Background(), forest_survey, before); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.ForestSurveyByID(context.Background(), forest_survey.ID)
	if err != nil || loaded.Status != domain.ForestSurveyPublished || loaded.SummaryLux != 30 {
		t.Fatalf("published forest_survey = %#v, %v", loaded, err)
	}
}

func TestPlanActionClaimAndLeaseOwnership(t *testing.T) {
	database, _ := openTestDatabase(t)
	set := seedForestAssetSet(t, database)
	store := database.Store()
	item := makeCase(t, "case_plan", set, storageNow, 4)
	if err := store.CreateCase(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	forest_survey := domain.NewForestSurvey("forest_survey_plan", item.ID, set.inspector.ID, storageNow)
	if err := store.CreateForestSurvey(context.Background(), forest_survey); err != nil {
		t.Fatal(err)
	}
	plan, err := domain.NewStewardshipPlan("plan_001", item.ID, forest_survey.ID, set.operator.ID, "Install a shield and lower the residential-facing lamp.", 1350, 8, storageNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreatePlan(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	actions := []domain.ForestAssetAction{
		{ID: "action_001", PlanID: plan.ID, ForestAssetID: set.forest_assets[0].ID, Type: domain.ActionInstallShield, Status: domain.ActionPending, TargetValue: "installed", Version: 1, CreatedAt: storageNow, UpdatedAt: storageNow},
		{ID: "action_002", PlanID: plan.ID, ForestAssetID: set.forest_assets[1].ID, Type: domain.ActionAdjustAngle, Status: domain.ActionPending, TargetValue: "-45", Version: 1, CreatedAt: storageNow.Add(time.Second), UpdatedAt: storageNow},
	}
	if err := store.CreateActions(context.Background(), actions); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimAction(context.Background(), actions[0].ID, set.operator.ID, storageNow, storageNow.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claimed.Status != domain.ActionClaimed || claimed.WorkerID == nil || *claimed.WorkerID != set.operator.ID || claimed.Version != 2 {
		t.Fatalf("claimed action = %#v", claimed)
	}
	if _, err := store.ClaimAction(context.Background(), actions[0].ID, set.admin.ID, storageNow.Add(30*time.Second), storageNow.Add(2*time.Minute)); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("claim during lease error = %v", err)
	}
	reclaimed, err := store.ClaimAction(context.Background(), actions[0].ID, set.admin.ID, storageNow.Add(2*time.Minute), storageNow.Add(3*time.Minute))
	if err != nil || reclaimed.WorkerID == nil || *reclaimed.WorkerID != set.admin.ID || reclaimed.Version != 3 {
		t.Fatalf("reclaimed action = %#v, %v", reclaimed, err)
	}
	if err := store.CompleteAction(context.Background(), reclaimed.ID, set.operator.ID, reclaimed.Version, storageNow.Add(2*time.Minute)); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("wrong-owner completion error = %v", err)
	}
	if err := store.CompleteAction(context.Background(), reclaimed.ID, set.admin.ID, reclaimed.Version, storageNow.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.ActionByID(context.Background(), reclaimed.ID)
	if err != nil || loaded.Status != domain.ActionCompleted || loaded.WorkerID != nil || loaded.Version != 4 {
		t.Fatalf("completed action = %#v, %v", loaded, err)
	}
}

func TestOutboxClaimRetryCompleteAndDeadLetter(t *testing.T) {
	database, _ := openTestDatabase(t)
	store := database.Store()
	jobs := []domain.OutboxJob{
		{ID: "job_001", Topic: "case.submitted", AggregateID: "case_001", Payload: []byte(`{"case_id":"case_001"}`), Status: domain.JobPending, AvailableAt: storageNow, MaxAttempts: 3, CreatedAt: storageNow, UpdatedAt: storageNow},
		{ID: "job_002", Topic: "case.submitted", AggregateID: "case_002", Payload: []byte(`{"case_id":"case_002"}`), Status: domain.JobPending, AvailableAt: storageNow.Add(time.Hour), MaxAttempts: 3, CreatedAt: storageNow, UpdatedAt: storageNow},
	}
	for _, job := range jobs {
		if err := store.EnqueueJob(context.Background(), job); err != nil {
			t.Fatal(err)
		}
	}
	workerOne, workerTwo := domain.ID("worker_001"), domain.ID("worker_002")
	claimed, err := store.ClaimJobs(context.Background(), workerOne, 10, storageNow, storageNow.Add(time.Minute))
	if err != nil || len(claimed) != 1 || claimed[0].ID != jobs[0].ID || claimed[0].Attempt != 1 {
		t.Fatalf("ClaimJobs() = %#v, %v", claimed, err)
	}
	other, err := store.ClaimJobs(context.Background(), workerTwo, 10, storageNow.Add(30*time.Second), storageNow.Add(2*time.Minute))
	if err != nil || len(other) != 0 {
		t.Fatalf("second ClaimJobs() = %#v, %v", other, err)
	}
	if err := store.RetryJob(context.Background(), jobs[0].ID, workerOne, "temporary delivery failure", storageNow.Add(2*time.Minute), storageNow.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimJobs(context.Background(), workerTwo, 10, storageNow.Add(3*time.Minute), storageNow.Add(4*time.Minute))
	if err != nil || len(claimed) != 1 || claimed[0].Attempt != 2 || claimed[0].LastError == "" {
		t.Fatalf("retry claim = %#v, %v", claimed, err)
	}
	if err := store.CompleteJob(context.Background(), jobs[0].ID, workerOne, storageNow.Add(3*time.Minute)); !errors.Is(err, domain.ErrLeaseLost) {
		t.Fatalf("wrong-owner CompleteJob() error = %v", err)
	}
	if err := store.DeadJob(context.Background(), jobs[0].ID, workerTwo, "invalid recipient", storageNow.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimJobs(context.Background(), workerOne, 10, storageNow.Add(4*time.Minute), storageNow.Add(5*time.Minute))
	if err != nil || len(claimed) != 0 {
		t.Fatalf("dead job was reclaimed: %#v, %v", claimed, err)
	}
}

func TestIdempotencyRecordRoundTripAndUniqueness(t *testing.T) {
	database, _ := openTestDatabase(t)
	store := database.Store()
	record := repository.IdempotencyRecord{Scope: "POST:/v1/cases:user:user_001", Key: "request-key-001", RequestHash: "hash-001", ResourceID: "case_001", ResponseCode: 201, ResponseBody: []byte(`{"id":"case_001"}`), CreatedAt: storageNow, ExpiresAt: storageNow.Add(48 * time.Hour)}
	if err := store.CreateIdempotencyRecord(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.IdempotencyRecord(context.Background(), record.Scope, record.Key)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RequestHash != record.RequestHash || loaded.ResourceID != record.ResourceID || string(loaded.ResponseBody) != string(record.ResponseBody) || !loaded.ExpiresAt.Equal(record.ExpiresAt) {
		t.Fatalf("loaded record = %#v", loaded)
	}
	if err := store.CreateIdempotencyRecord(context.Background(), record); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate idempotency record error = %v", err)
	}
}

func TestForeignKeyAndUniqueViolationsMapToConflict(t *testing.T) {
	database, _ := openTestDatabase(t)
	set := seedForestAssetSet(t, database)
	if err := database.Store().CreateUser(context.Background(), set.admin); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("duplicate user error = %v", err)
	}
	invalid, err := domain.NewForestSite("forest_site_invalid", "Unknown Operator ForestSite", "missing_operator", "Luohu", "Asia/Shanghai", 1350, storageNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Store().CreateForestSite(context.Background(), invalid); err == nil {
		t.Fatal("foreign-key violating forest_site was inserted")
	}
}
