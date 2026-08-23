package service_test

import (
	"context"
	"errors"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/service"
	"testing"
)

func TestOperatorCannotListAssetsFromAnotherSite(t *testing.T) {
	f := newServiceForestAsset(t)
	other, e := f.service.CreateForestSite(context.Background(), f.admin, service.CreateForestSiteInput{Name: "Second Forest", OperatorID: f.admin.User.ID, Address: "Remote", Timezone: "Asia/Shanghai", CutoffMinute: 1200, Latitude: 31, Longitude: 121})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.service.CreateForestAsset(context.Background(), f.admin, service.CreateForestAssetInput{ForestSiteID: other.ID, Label: "Remote row", RowNumber: 1, Orientation: "north", Angle: 0})
	if e != nil {
		t.Fatal(e)
	}
	items, e := f.service.ListForestAssets(context.Background(), f.operator, other.ID)
	if !errors.Is(e, domain.ErrForbidden) {
		t.Fatalf("cross-site assets leaked: items=%#v err=%v", items, e)
	}
}
