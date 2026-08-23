package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func TestPatrolAlertPublishesOutboxJob(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	if err := f.service.PublishPatrolAlert(context.Background(), f.officer, item.ID); err != nil {
		t.Fatal(err)
	}
	jobs, err := f.database.Store().ClaimJobs(context.Background(), domain.ID("alert-worker"), 20, f.clock.Now(), f.clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, job := range jobs {
		if job.Topic == "patrol.alert" && job.AggregateID == item.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("patrol alert event committed without an outbox job")
	}
}
