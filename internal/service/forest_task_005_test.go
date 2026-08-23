package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func TestCancelledPatrolNoteDoesNotPersist(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	events, err := f.database.Store().ListCaseEvents(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = f.service.RecordPatrolNote(ctx, f.officer, item.ID, "巡护记录已完成现场复核")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled note error = %v", err)
	}
	after, err := f.database.Store().ListCaseEvents(context.Background(), item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(events) {
		t.Fatalf("cancelled request persisted a patrol note: before=%d after=%d", len(events), len(after))
	}
	_ = domain.CaseSubmitted
}
