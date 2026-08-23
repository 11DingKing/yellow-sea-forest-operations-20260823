package service_test

import (
	"context"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"testing"
)

func TestPatrolCutoffUpdatePersistsAcrossReload(t *testing.T) {
	f := newServiceForestAsset(t)
	if err := f.service.SetPatrolCutoff(context.Background(), f.admin, f.forest_site.ID, 1200); err != nil {
		t.Fatal(err)
	}
	site, err := f.database.Store().ForestSiteByID(context.Background(), f.forest_site.ID)
	if err != nil {
		t.Fatal(err)
	}
	if site.CutoffMinute != 1200 {
		t.Fatalf("patrol cutoff did not persist: %d", site.CutoffMinute)
	}
	_ = domain.ForestSiteActive
}
