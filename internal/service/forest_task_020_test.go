package service_test

import (
	"context"
	"testing"
)

func TestCaseAssignmentWritesAuditEvent(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	audits, e := f.database.Store().ListAudit(context.Background(), "forest_case", item.ID, 20)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, a := range audits {
		if a.Action == "case.assign" {
			found = true
		}
	}
	if !found {
		t.Fatalf("assignment audit missing: %#v", audits)
	}
}
