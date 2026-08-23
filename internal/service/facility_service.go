package service

import (
	"context"
	"fmt"
	"math"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type CreateForestSiteInput struct {
	Name         string
	OperatorID   domain.ID
	Address      string
	Timezone     string
	CutoffMinute int
	Latitude     float64
	Longitude    float64
}

func (s *Service) CreateForestSite(ctx context.Context, principal Principal, input CreateForestSiteInput) (domain.ForestSite, error) {
	if err := requireAction(principal, "admin:manage"); err != nil {
		return domain.ForestSite{}, err
	}
	operator, err := s.uow.Store().UserByID(ctx, input.OperatorID)
	if err != nil {
		return domain.ForestSite{}, fmt.Errorf("operator: %w", err)
	}
	if !operator.Active || (operator.Role != domain.RoleOperator && operator.Role != domain.RoleAdmin) {
		return domain.ForestSite{}, domain.FieldError{Field: "operator_id", Message: "must reference an active operator"}
	}
	id, err := domain.NewID("forest_site")
	if err != nil {
		return domain.ForestSite{}, err
	}
	now := s.clock.Now()
	forest_site, err := domain.NewForestSite(id, input.Name, input.OperatorID, input.Address, input.Timezone, input.CutoffMinute, now)
	if err != nil {
		return domain.ForestSite{}, err
	}
	if math.IsNaN(input.Latitude) || math.IsInf(input.Latitude, 0) || math.IsNaN(input.Longitude) || math.IsInf(input.Longitude, 0) || input.Latitude < -90 || input.Latitude > 90 || input.Longitude < -180 || input.Longitude > 180 {
		return domain.ForestSite{}, domain.FieldError{Field: "coordinates", Message: "are outside geographic bounds"}
	}
	forest_site.Latitude = input.Latitude
	forest_site.Longitude = input.Longitude
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		if err := store.CreateForestSite(ctx, forest_site); err != nil {
			return err
		}
		audit, err := newAudit(principal.User.ID, "forest_site.create", "forest_site", forest_site.ID, "success", map[string]string{"operator_id": input.OperatorID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendAudit(ctx, audit)
	})
	return forest_site, err
}

type CreateZoneInput struct {
	Name           string
	Address        string
	ContactUserID  domain.ID
	WindowCount    int
	SensitivityLux float64
}

func (s *Service) CreateForestParcel(ctx context.Context, principal Principal, input CreateZoneInput) (domain.ForestParcel, error) {
	if err := requireAction(principal, "admin:manage"); err != nil {
		return domain.ForestParcel{}, err
	}
	contact, err := s.uow.Store().UserByID(ctx, input.ContactUserID)
	if err != nil {
		return domain.ForestParcel{}, fmt.Errorf("contact: %w", err)
	}
	if !contact.Active || (contact.Role != domain.RoleResidentLiaison && contact.Role != domain.RoleAdmin) {
		return domain.ForestParcel{}, domain.FieldError{Field: "contact_user_id", Message: "must reference an active resident liaison"}
	}
	id, err := domain.NewID("zone")
	if err != nil {
		return domain.ForestParcel{}, err
	}
	now := s.clock.Now()
	zone, err := domain.NewForestParcel(id, input.Name, input.Address, input.ContactUserID, input.WindowCount, input.SensitivityLux, now)
	if err != nil {
		return domain.ForestParcel{}, err
	}
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		if err := store.CreateZone(ctx, zone); err != nil {
			return err
		}
		audit, err := newAudit(principal.User.ID, "zone.create", "forest_parcel", zone.ID, "success", map[string]string{"contact_user_id": contact.ID.String()}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendAudit(ctx, audit)
	})
	return zone, err
}

type CreateForestAssetInput struct {
	ForestSiteID domain.ID
	Label        string
	RowNumber    int
	Orientation  string
	Angle        float64
}

func (s *Service) CreateForestAsset(ctx context.Context, principal Principal, input CreateForestAssetInput) (domain.ForestAsset, error) {
	if err := requireAction(principal, "admin:manage"); err != nil {
		return domain.ForestAsset{}, err
	}
	if _, err := s.uow.Store().ForestSiteByID(ctx, input.ForestSiteID); err != nil {
		return domain.ForestAsset{}, fmt.Errorf("forest_site: %w", err)
	}
	id, err := domain.NewID("forest_asset")
	if err != nil {
		return domain.ForestAsset{}, err
	}
	now := s.clock.Now()
	forest_asset, err := domain.NewForestAsset(id, input.ForestSiteID, input.Label, input.RowNumber, input.Orientation, input.Angle, now)
	if err != nil {
		return domain.ForestAsset{}, err
	}
	err = s.uow.WithinTx(ctx, func(store repository.Store) error {
		if err := store.CreateForestAsset(ctx, forest_asset); err != nil {
			return err
		}
		audit, err := newAudit(principal.User.ID, "forest_asset.create", "forest_asset", forest_asset.ID, "success", map[string]string{"forest_site_id": forest_asset.ForestSiteID.String(), "row": fmt.Sprint(forest_asset.RowNumber)}, requestID(ctx), now)
		if err != nil {
			return err
		}
		return store.AppendAudit(ctx, audit)
	})
	return forest_asset, err
}

func (s *Service) ListForestAssets(ctx context.Context, principal Principal, forest_siteID domain.ID) ([]domain.ForestAsset, error) {
	if err := requireAction(principal, "case:view"); err != nil {
		return nil, err
	}
	items, err := s.uow.Store().ListForestAssets(ctx, forest_siteID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.ForestAsset, len(items))
	copy(result, items)
	return result, nil
}

func (s *Service) SetPatrolCutoff(ctx context.Context, principal Principal, siteID domain.ID, minute int) error {
	if err := requireAction(principal, "admin:manage"); err != nil {
		return err
	}
	site, err := s.uow.Store().ForestSiteByID(ctx, siteID)
	if err != nil {
		return err
	}
	if err := site.SetCutoff(minute, site.Version, s.clock.Now()); err != nil {
		return err
	}
	return nil
}
