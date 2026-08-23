package service_test

import (
	"context"
	"testing"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
)

func TestPatrolMeasurementBatchRollsBackOnDuplicate(t *testing.T) {
	f := newServiceForestAsset(t)
	item := f.submitAndAssign(t)
	survey, err := f.service.StartForestSurvey(context.Background(), f.inspector, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = f.service.RecordPatrolMeasurements(context.Background(), f.inspector, survey.ID, []service.AddReadingInput{
		{Position: "north tree line", Lux: 12, Sequence: 1},
		{Position: "south tree line", Lux: 18, Sequence: 1},
	})
	if err == nil {
		t.Fatal("duplicate patrol measurement batch unexpectedly succeeded")
	}
	readings, err := f.database.Store().ListReadings(context.Background(), survey.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(readings) != 0 {
		t.Fatalf("failed batch left %d patrol measurements", len(readings))
	}
}
