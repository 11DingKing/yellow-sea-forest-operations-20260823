package service_test

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"testing"
)

func TestResidentPatrolListStaysWithinAssignedParcel(t *testing.T) {
	f := newServiceForestAsset(t)
	if _, err := f.service.SubmitCase(context.Background(), f.liaison, service.SubmitCaseInput{ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "assigned parcel issue", Description: "issue in assigned forest parcel", Priority: 3, IdempotencyKey: "assigned-parcel-issue"}); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.ListResidentPatrolCases(context.Background(), f.liaison, domain.PageRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 {
		t.Fatalf("resident list crossed parcel boundary: total=%d", page.Total)
	}
}
