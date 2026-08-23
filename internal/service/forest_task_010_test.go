package service_test

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"testing"
)

func TestPatrolCaseSearchReachesPersistence(t *testing.T) {
	f := newServiceForestAsset(t)
	if _, err := f.service.SubmitCase(context.Background(), f.liaison, service.SubmitCaseInput{ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "searchable patrol alert", Description: "forest patrol report with unique keyword", Priority: 3, IdempotencyKey: "searchable-patrol"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SubmitCase(context.Background(), f.liaison, service.SubmitCaseInput{ForestSiteID: f.forest_site.ID, ForestParcelID: f.zone.ID, Title: "other patrol alert", Description: "forest patrol report with another keyword", Priority: 3, IdempotencyKey: "other-patrol"}); err != nil {
		t.Fatal(err)
	}
	page, err := f.service.ListPatrolCases(context.Background(), f.admin, domain.CaseFilter{}.WithSearch("unique keyword"), domain.PageRequest{Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("search filter was dropped, total=%d", page.Total)
	}
}
