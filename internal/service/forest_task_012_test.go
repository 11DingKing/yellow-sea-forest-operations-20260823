package service_test

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"testing"
)

func TestPatrolReadingCountMatchesPersistedRows(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey, err := f.service.StartForestSurvey(context.Background(), f.inspector, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		if _, err := f.service.AddReading(context.Background(), f.inspector, service.AddReadingInput{ForestSurveyID: survey.ID, Position: "row", Lux: float64(i), Sequence: i}); err != nil {
			t.Fatal(err)
		}
	}
	count, err := f.service.CountPatrolReadings(context.Background(), f.inspector, survey.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("patrol reading count=%d want 2", count)
	}
}
