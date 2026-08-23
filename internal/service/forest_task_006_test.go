package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func TestPatrolCaseNotFoundRetainsDomainError(t *testing.T) {
	f := newServiceForestAsset(t)
	_, err := f.service.GetPatrolCase(context.Background(), f.admin, domain.ID("case_missing_patrol"))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing patrol case lost not-found identity: %v", err)
	}
}
