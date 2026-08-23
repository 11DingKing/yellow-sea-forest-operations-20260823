package sqlite

import (
	"context"
	"fmt"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) CreateForestSite(ctx context.Context, forest_site domain.ForestSite) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO forest_sites(id,name,operator_id,address,timezone,status,latitude,longitude,cutoff_minute,version,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, forest_site.ID, forest_site.Name, forest_site.OperatorID, forest_site.Address, forest_site.Timezone, forest_site.Status, forest_site.Latitude, forest_site.Longitude, forest_site.CutoffMinute, forest_site.Version, formatTime(forest_site.CreatedAt), formatTime(forest_site.UpdatedAt))
	return mapError("create forest_site", err)
}

func scanForestSite(row interface{ Scan(...any) error }) (domain.ForestSite, error) {
	var forest_site domain.ForestSite
	var id, operatorID, status, created, updated string
	if err := row.Scan(&id, &forest_site.Name, &operatorID, &forest_site.Address, &forest_site.Timezone, &status, &forest_site.Latitude, &forest_site.Longitude, &forest_site.CutoffMinute, &forest_site.Version, &created, &updated); err != nil {
		return domain.ForestSite{}, err
	}
	forest_site.ID = domain.ID(id)
	forest_site.OperatorID = domain.ID(operatorID)
	forest_site.Status = domain.ForestSiteStatus(status)
	var err error
	if forest_site.CreatedAt, err = parseTime(created); err != nil {
		return domain.ForestSite{}, fmt.Errorf("parse forest_site created_at: %w", err)
	}
	if forest_site.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ForestSite{}, fmt.Errorf("parse forest_site updated_at: %w", err)
	}
	return forest_site, nil
}

func (s *store) ForestSiteByID(ctx context.Context, id domain.ID) (domain.ForestSite, error) {
	forest_site, err := scanForestSite(s.queryer.QueryRowContext(ctx, `
		SELECT id,name,operator_id,address,timezone,status,latitude,longitude,cutoff_minute,version,created_at,updated_at
		FROM forest_sites WHERE id=?`, id))
	return forest_site, mapError("get forest_site", err)
}

func (s *store) UpdateForestSite(ctx context.Context, forest_site domain.ForestSite, expectedVersion int64) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_sites SET status=?,latitude=?,longitude=?,cutoff_minute=?,version=?,updated_at=?
		WHERE id=? AND version=?`, forest_site.Status, forest_site.Latitude, forest_site.Longitude, forest_site.CutoffMinute, forest_site.Version, formatTime(forest_site.UpdatedAt), forest_site.ID, expectedVersion)
	if err != nil {
		return mapError("update forest_site", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update forest_site rows: %w", err)
	}
	if rows != 1 {
		return domain.VersionConflictError{Entity: "forest_site", Expected: expectedVersion, Actual: forest_site.Version}
	}
	return nil
}

func (s *store) CreateZone(ctx context.Context, zone domain.ForestParcel) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO forest_parcels(id,name,address,contact_user_id,window_count,sensitivity_lux,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, zone.ID, zone.Name, zone.Address, zone.ContactUserID, zone.WindowCount, zone.SensitivityLux, formatTime(zone.CreatedAt), formatTime(zone.UpdatedAt))
	return mapError("create residential zone", err)
}

func scanZone(row interface{ Scan(...any) error }) (domain.ForestParcel, error) {
	var zone domain.ForestParcel
	var id, contactID, created, updated string
	if err := row.Scan(&id, &zone.Name, &zone.Address, &contactID, &zone.WindowCount, &zone.SensitivityLux, &created, &updated); err != nil {
		return domain.ForestParcel{}, err
	}
	zone.ID = domain.ID(id)
	zone.ContactUserID = domain.ID(contactID)
	var err error
	if zone.CreatedAt, err = parseTime(created); err != nil {
		return domain.ForestParcel{}, fmt.Errorf("parse zone created_at: %w", err)
	}
	if zone.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ForestParcel{}, fmt.Errorf("parse zone updated_at: %w", err)
	}
	return zone, nil
}

func (s *store) ZoneByID(ctx context.Context, id domain.ID) (domain.ForestParcel, error) {
	zone, err := scanZone(s.queryer.QueryRowContext(ctx, `
		SELECT id,name,address,contact_user_id,window_count,sensitivity_lux,created_at,updated_at
		FROM forest_parcels WHERE id=?`, id))
	return zone, mapError("get residential zone", err)
}

func scanForestAsset(row interface{ Scan(...any) error }) (domain.ForestAsset, error) {
	var forest_asset domain.ForestAsset
	var id, forest_siteID, updated string
	if err := row.Scan(&id, &forest_siteID, &forest_asset.Label, &forest_asset.RowNumber, &forest_asset.Orientation, &forest_asset.Enabled, &forest_asset.Shielded, &forest_asset.AngleDegrees, &forest_asset.Version, &updated); err != nil {
		return domain.ForestAsset{}, err
	}
	forest_asset.ID = domain.ID(id)
	forest_asset.ForestSiteID = domain.ID(forest_siteID)
	var err error
	forest_asset.UpdatedAt, err = parseTime(updated)
	return forest_asset, err
}

func (s *store) CreateForestAsset(ctx context.Context, forest_asset domain.ForestAsset) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO forest_assets(id,forest_site_id,label,row_number,orientation,enabled,shielded,angle_degrees,version,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, forest_asset.ID, forest_asset.ForestSiteID, forest_asset.Label, forest_asset.RowNumber, forest_asset.Orientation, forest_asset.Enabled, forest_asset.Shielded, forest_asset.AngleDegrees, forest_asset.Version, formatTime(forest_asset.UpdatedAt))
	return mapError("create forest_asset", err)
}

func (s *store) ForestAssetByID(ctx context.Context, id domain.ID) (domain.ForestAsset, error) {
	forest_asset, err := scanForestAsset(s.queryer.QueryRowContext(ctx, `
		SELECT id,forest_site_id,label,row_number,orientation,enabled,shielded,angle_degrees,version,updated_at
		FROM forest_assets WHERE id=?`, id))
	return forest_asset, mapError("get forest_asset", err)
}

func (s *store) ListForestAssets(ctx context.Context, forest_siteID domain.ID) ([]domain.ForestAsset, error) {
	rows, err := s.queryer.QueryContext(ctx, `
		SELECT id,forest_site_id,label,row_number,orientation,enabled,shielded,angle_degrees,version,updated_at
		FROM forest_assets WHERE forest_site_id=? ORDER BY row_number,label`, forest_siteID)
	if err != nil {
		return nil, mapError("list forest_assets", err)
	}
	defer rows.Close()
	forest_assets := make([]domain.ForestAsset, 0)
	for rows.Next() {
		forest_asset, scanErr := scanForestAsset(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan forest_asset: %w", scanErr)
		}
		forest_assets = append(forest_assets, forest_asset)
	}
	return forest_assets, rows.Err()
}

func (s *store) UpdateForestAsset(ctx context.Context, forest_asset domain.ForestAsset, expectedVersion int64) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_assets SET enabled=?,shielded=?,angle_degrees=?,version=version+1,updated_at=?
		WHERE id=? AND version=?`, forest_asset.Enabled, forest_asset.Shielded, forest_asset.AngleDegrees, formatTime(forest_asset.UpdatedAt), forest_asset.ID, expectedVersion)
	if err != nil {
		return mapError("update forest_asset", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.VersionConflictError{Entity: "forest_asset", Expected: expectedVersion, Actual: forest_asset.Version}
	}
	return nil
}
