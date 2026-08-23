package domain

import (
	"errors"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 8, 20, 20, 30, 0, 0, time.FixedZone("CST", 8*60*60))
}

func newCaseForTest(t *testing.T) ForestCase {
	t.Helper()
	item, err := NewForestCase("case_001", "forest_site_001", "zone_001", "user_001", "Football field glare", "Strong white light reaches residential windows every night.", 4, fixedTime())
	if err != nil {
		t.Fatalf("NewForestCase() error = %v", err)
	}
	return item
}

func TestNewForestCaseNormalizesAndInitializes(t *testing.T) {
	now := fixedTime()
	item, err := NewForestCase("case_001", "forest_site_001", "zone_001", "user_001", "  Night glare report  ", "  Light reaches balconies after the field opens.  ", 3, now)
	if err != nil {
		t.Fatalf("NewForestCase() error = %v", err)
	}
	if item.Title != "Night glare report" {
		t.Fatalf("Title = %q", item.Title)
	}
	if item.Description != "Light reaches balconies after the field opens." {
		t.Fatalf("Description = %q", item.Description)
	}
	if item.Status != CaseSubmitted || item.Version != 1 {
		t.Fatalf("initial status/version = %s/%d", item.Status, item.Version)
	}
	if !item.SubmittedAt.Equal(now.UTC()) || !item.UpdatedAt.Equal(now.UTC()) {
		t.Fatalf("timestamps were not normalized to UTC: %#v", item)
	}
}

func TestNewForestCaseRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		description string
		priority    int
		field       string
	}{
		{name: "short title", title: "bad", description: "description long enough", priority: 3, field: "title"},
		{name: "long title", title: string(make([]byte, 161)), description: "description long enough", priority: 3, field: "title"},
		{name: "short description", title: "valid title", description: "too short", priority: 3, field: "description"},
		{name: "priority below range", title: "valid title", description: "description long enough", priority: 0, field: "priority"},
		{name: "priority above range", title: "valid title", description: "description long enough", priority: 6, field: "priority"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewForestCase("case_001", "forest_site_001", "zone_001", "user_001", test.title, test.description, test.priority, fixedTime())
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("error = %v, want validation", err)
			}
			var fieldErr FieldError
			if !errors.As(err, &fieldErr) || fieldErr.Field != test.field {
				t.Fatalf("field error = %#v, want %q", fieldErr, test.field)
			}
		})
	}
}

func TestForestCaseFullStateMachine(t *testing.T) {
	item := newCaseForTest(t)
	owner := ID("officer_001")
	now := fixedTime().Add(time.Minute)
	if err := item.Assign(owner, 1, now); err != nil {
		t.Fatalf("Assign() error = %v", err)
	}
	if item.Status != CaseTriaged || item.OwnerID == nil || *item.OwnerID != owner || item.Version != 2 {
		t.Fatalf("assigned case = %#v", item)
	}
	steps := []CaseStatus{CaseSurveying, CaseStewardshipPlanned, CaseExecuting, CaseVerifying, CaseResolved}
	for index, status := range steps {
		transitionAt := now.Add(time.Duration(index+1) * time.Minute)
		if err := item.Transition(status, transitionAt); err != nil {
			t.Fatalf("Transition(%s) error = %v", status, err)
		}
		if item.Status != status {
			t.Fatalf("status = %s, want %s", item.Status, status)
		}
		if !item.UpdatedAt.Equal(transitionAt.UTC()) {
			t.Fatalf("updated_at = %s, want %s", item.UpdatedAt, transitionAt.UTC())
		}
	}
	if item.ResolvedAt == nil || item.ReopenUntil == nil {
		t.Fatalf("resolved case has no feedback window: %#v", item)
	}
	if duration := item.ReopenUntil.Sub(*item.ResolvedAt); duration != 14*24*time.Hour {
		t.Fatalf("feedback window = %s", duration)
	}
	if !item.CanReopen(item.ReopenUntil.Add(-time.Second)) {
		t.Fatal("CanReopen() = false during feedback window")
	}
	if item.CanReopen(item.ReopenUntil.Add(time.Second)) {
		t.Fatal("CanReopen() = true after feedback window")
	}
	if err := item.Transition(CaseReopened, item.ReopenUntil.Add(-time.Second)); err != nil {
		t.Fatalf("Transition(reopened) error = %v", err)
	}
	if item.ResolvedAt != nil || item.ReopenUntil != nil {
		t.Fatalf("resolution window survived reopen: resolved=%v reopen_until=%v", item.ResolvedAt, item.ReopenUntil)
	}
}

func TestForestCaseRejectsIllegalTransitions(t *testing.T) {
	tests := []struct {
		from CaseStatus
		to   CaseStatus
	}{
		{CaseSubmitted, CaseExecuting},
		{CaseTriaged, CaseResolved},
		{CaseSurveying, CaseVerifying},
		{CaseStewardshipPlanned, CaseResolved},
		{CaseExecuting, CaseResolved},
		{CaseResolved, CaseSurveying},
	}
	for _, test := range tests {
		t.Run(string(test.from)+"_to_"+string(test.to), func(t *testing.T) {
			item := newCaseForTest(t)
			item.Status = test.from
			before := item.Clone()
			err := item.Transition(test.to, fixedTime().Add(time.Hour))
			if !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("error = %v, want invalid transition", err)
			}
			if item.Status != before.Status || item.Version != before.Version || !item.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("illegal transition mutated case: before=%#v after=%#v", before, item)
			}
		})
	}
}

func TestForestCaseAssignmentUsesOptimisticVersion(t *testing.T) {
	item := newCaseForTest(t)
	err := item.Assign("officer_001", 99, fixedTime().Add(time.Minute))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	var versionErr VersionConflictError
	if !errors.As(err, &versionErr) {
		t.Fatalf("error type = %T", err)
	}
	if versionErr.Expected != 99 || versionErr.Actual != 1 {
		t.Fatalf("version error = %#v", versionErr)
	}
	if item.OwnerID != nil || item.Status != CaseSubmitted || item.Version != 1 {
		t.Fatalf("version conflict mutated case: %#v", item)
	}
}

func TestForestCaseAssignmentCannotChangeOwner(t *testing.T) {
	item := newCaseForTest(t)
	if err := item.Assign("officer_001", 1, fixedTime()); err != nil {
		t.Fatal(err)
	}
	err := item.Assign("officer_002", item.Version, fixedTime().Add(time.Minute))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	if item.OwnerID == nil || *item.OwnerID != "officer_001" {
		t.Fatalf("owner changed after conflict: %#v", item.OwnerID)
	}
}

func TestForestCaseCloneOwnsPointerFields(t *testing.T) {
	item := newCaseForTest(t)
	owner := ID("officer_001")
	resolved := fixedTime()
	reopen := resolved.Add(14 * 24 * time.Hour)
	item.OwnerID = &owner
	item.ResolvedAt = &resolved
	item.ReopenUntil = &reopen
	clone := item.Clone()
	*clone.OwnerID = "officer_other"
	*clone.ResolvedAt = resolved.Add(time.Hour)
	*clone.ReopenUntil = reopen.Add(time.Hour)
	if *item.OwnerID != owner || !item.ResolvedAt.Equal(resolved) || !item.ReopenUntil.Equal(reopen) {
		t.Fatalf("clone aliases source pointers: source=%#v clone=%#v", item, clone)
	}
}

func TestCaseEventCloneOwnsPayload(t *testing.T) {
	event := CaseEvent{ID: "event_001", Payload: map[string]string{"result": "passed"}}
	clone := event.Clone()
	clone.Payload["result"] = "failed"
	clone.Payload["extra"] = "value"
	if event.Payload["result"] != "passed" {
		t.Fatalf("source payload changed: %#v", event.Payload)
	}
	if _, exists := event.Payload["extra"]; exists {
		t.Fatalf("clone added key to source: %#v", event.Payload)
	}
}

func TestPageRequestNormalizeAndOffset(t *testing.T) {
	tests := []struct {
		name  string
		input PageRequest
		want  PageRequest
	}{
		{name: "defaults", input: PageRequest{}, want: PageRequest{Page: 1, PageSize: 20, Sort: "submitted_at"}},
		{name: "caps size", input: PageRequest{Page: 3, PageSize: 500, Sort: "PRIORITY", Desc: true}, want: PageRequest{Page: 3, PageSize: 100, Sort: "priority", Desc: true}},
		{name: "unknown sort", input: PageRequest{Page: 2, PageSize: 10, Sort: "unknown"}, want: PageRequest{Page: 2, PageSize: 10, Sort: "submitted_at"}},
	}
	allowed := map[string]bool{"submitted_at": true, "priority": true}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.input.Normalize(allowed, "submitted_at")
			if got != test.want {
				t.Fatalf("Normalize() = %#v, want %#v", got, test.want)
			}
			if got.Offset() != (got.Page-1)*got.PageSize {
				t.Fatalf("Offset() = %d", got.Offset())
			}
		})
	}
}

func TestCaseFilterCloneOwnsSlicesAndPointers(t *testing.T) {
	forest_site, zone, owner := ID("forest_site_001"), ID("zone_001"), ID("owner_001")
	createdFrom, createdTo := fixedTime(), fixedTime().Add(time.Hour)
	filter := CaseFilter{Statuses: []CaseStatus{CaseSubmitted, CaseTriaged}, ForestSiteID: &forest_site, ZoneID: &zone, OwnerID: &owner, CreatedFrom: &createdFrom, CreatedTo: &createdTo}
	clone := filter.Clone()
	clone.Statuses[0] = CaseResolved
	*clone.ForestSiteID = "forest_site_002"
	*clone.ZoneID = "zone_002"
	*clone.OwnerID = "owner_002"
	*clone.CreatedFrom = clone.CreatedFrom.Add(time.Hour)
	*clone.CreatedTo = clone.CreatedTo.Add(time.Hour)
	if filter.Statuses[0] != CaseSubmitted || *filter.ForestSiteID != forest_site || *filter.ZoneID != zone || *filter.OwnerID != owner || !filter.CreatedFrom.Equal(createdFrom) || !filter.CreatedTo.Equal(createdTo) {
		t.Fatalf("Clone() aliases source: source=%#v clone=%#v", filter, clone)
	}
}
