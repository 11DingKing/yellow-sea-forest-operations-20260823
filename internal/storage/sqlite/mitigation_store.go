package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) CreatePlan(ctx context.Context, plan domain.StewardshipPlan) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO forest_plans(id,case_id,forest_survey_id,created_by,status,description,cutoff_minute,expected_max_lux,version,created_at,updated_at,approved_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, plan.ID, plan.CaseID, plan.ForestSurveyID, plan.CreatedBy, plan.Status, plan.Description, plan.CutoffMinute, plan.ExpectedMaxLux, plan.Version, formatTime(plan.CreatedAt), formatTime(plan.UpdatedAt), nullableTime(plan.ApprovedAt))
	return mapError("create mitigation plan", err)
}

func scanPlan(row interface{ Scan(...any) error }) (domain.StewardshipPlan, error) {
	var plan domain.StewardshipPlan
	var id, caseID, forest_surveyID, createdBy, status, created, updated string
	var approved sql.NullString
	if err := row.Scan(&id, &caseID, &forest_surveyID, &createdBy, &status, &plan.Description, &plan.CutoffMinute, &plan.ExpectedMaxLux, &plan.Version, &created, &updated, &approved); err != nil {
		return domain.StewardshipPlan{}, err
	}
	plan.ID, plan.CaseID, plan.ForestSurveyID, plan.CreatedBy = domain.ID(id), domain.ID(caseID), domain.ID(forest_surveyID), domain.ID(createdBy)
	plan.Status = domain.PlanStatus(status)
	var err error
	if plan.CreatedAt, err = parseTime(created); err != nil {
		return domain.StewardshipPlan{}, err
	}
	if plan.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.StewardshipPlan{}, err
	}
	if approved.Valid {
		value, parseErr := parseTime(approved.String)
		if parseErr != nil {
			return domain.StewardshipPlan{}, parseErr
		}
		plan.ApprovedAt = &value
	}
	return plan, nil
}

const selectPlan = `SELECT id,case_id,forest_survey_id,created_by,status,description,cutoff_minute,expected_max_lux,version,created_at,updated_at,approved_at FROM forest_plans`

func (s *store) PlanByID(ctx context.Context, id domain.ID) (domain.StewardshipPlan, error) {
	plan, err := scanPlan(s.queryer.QueryRowContext(ctx, selectPlan+` WHERE id=?`, id))
	return plan, mapError("get mitigation plan", err)
}

func (s *store) PlanForCase(ctx context.Context, caseID domain.ID) (domain.StewardshipPlan, error) {
	plan, err := scanPlan(s.queryer.QueryRowContext(ctx, selectPlan+` WHERE case_id=? ORDER BY created_at DESC LIMIT 1`, caseID))
	return plan, mapError("get case mitigation plan", err)
}

func (s *store) UpdatePlan(ctx context.Context, plan domain.StewardshipPlan, expectedVersion int64) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_plans SET status=?,version=?,updated_at=?,approved_at=? WHERE id=? AND version=?`, plan.Status, plan.Version, formatTime(plan.UpdatedAt), nullableTime(plan.ApprovedAt), plan.ID, expectedVersion)
	if err != nil {
		return mapError("update mitigation plan", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.VersionConflictError{Entity: "stewardship_plan", Expected: expectedVersion, Actual: plan.Version}
	}
	return nil
}

func (s *store) CreateActions(ctx context.Context, actions []domain.ForestAssetAction) error {
	for _, action := range actions {
		_, err := s.queryer.ExecContext(ctx, `
			INSERT INTO forest_asset_actions(id,plan_id,forest_asset_id,action_type,status,target_value,worker_id,lease_expires_at,attempt,last_error,version,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, action.ID, action.PlanID, action.ForestAssetID, action.Type, action.Status, action.TargetValue, nullableID(action.WorkerID), nullableTime(action.LeaseExpiresAt), action.Attempt, action.LastError, action.Version, formatTime(action.CreatedAt), formatTime(action.UpdatedAt))
		if err != nil {
			return mapError("create forest_asset action", err)
		}
	}
	return nil
}

func scanAction(row interface{ Scan(...any) error }) (domain.ForestAssetAction, error) {
	var action domain.ForestAssetAction
	var id, planID, forest_assetID, kind, status, created, updated string
	var worker, lease sql.NullString
	if err := row.Scan(&id, &planID, &forest_assetID, &kind, &status, &action.TargetValue, &worker, &lease, &action.Attempt, &action.LastError, &action.Version, &created, &updated); err != nil {
		return domain.ForestAssetAction{}, err
	}
	action.ID, action.PlanID, action.ForestAssetID = domain.ID(id), domain.ID(planID), domain.ID(forest_assetID)
	action.Type, action.Status = domain.ForestAssetActionType(kind), domain.ActionStatus(status)
	if worker.Valid {
		value := domain.ID(worker.String)
		action.WorkerID = &value
	}
	if lease.Valid {
		value, err := parseTime(lease.String)
		if err != nil {
			return domain.ForestAssetAction{}, err
		}
		action.LeaseExpiresAt = &value
	}
	var err error
	if action.CreatedAt, err = parseTime(created); err != nil {
		return domain.ForestAssetAction{}, err
	}
	if action.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ForestAssetAction{}, err
	}
	return action, nil
}

const selectAction = `SELECT id,plan_id,forest_asset_id,action_type,status,target_value,worker_id,lease_expires_at,attempt,last_error,version,created_at,updated_at FROM forest_asset_actions`

func (s *store) ActionByID(ctx context.Context, id domain.ID) (domain.ForestAssetAction, error) {
	action, err := scanAction(s.queryer.QueryRowContext(ctx, selectAction+` WHERE id=?`, id))
	return action.Clone(), mapError("get forest_asset action", err)
}

func (s *store) ListActions(ctx context.Context, planID domain.ID) ([]domain.ForestAssetAction, error) {
	rows, err := s.queryer.QueryContext(ctx, selectAction+` WHERE plan_id=? ORDER BY created_at,id`, planID)
	if err != nil {
		return nil, mapError("list forest_asset actions", err)
	}
	defer rows.Close()
	actions := make([]domain.ForestAssetAction, 0)
	for rows.Next() {
		action, scanErr := scanAction(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		actions = append(actions, action.Clone())
	}
	return actions, rows.Err()
}

func (s *store) ClaimAction(ctx context.Context, id, workerID domain.ID, now, leaseUntil time.Time) (domain.ForestAssetAction, error) {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_asset_actions SET status='claimed',worker_id=?,lease_expires_at=?,attempt=attempt+1,version=version+1,updated_at=?
		WHERE id=? AND (status='pending' OR (status='claimed' AND lease_expires_at<=?))`, workerID, formatTime(leaseUntil), formatTime(now), id, formatTime(now))
	if err != nil {
		return domain.ForestAssetAction{}, mapError("claim forest_asset action", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return domain.ForestAssetAction{}, err
	}
	if rows != 1 {
		return domain.ForestAssetAction{}, domain.ConflictError{Resource: "forest_asset_action", Reason: "not claimable"}
	}
	action, err := scanAction(s.queryer.QueryRowContext(ctx, selectAction+` WHERE id=?`, id))
	return action, mapError("read claimed forest_asset action", err)
}

func (s *store) CompleteAction(ctx context.Context, id, workerID domain.ID, expectedVersion int64, now time.Time) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_asset_actions SET status='completed',worker_id=NULL,lease_expires_at=NULL,version=version+1,updated_at=?
		WHERE id=? AND worker_id=? AND status='claimed' AND version=? AND lease_expires_at>?`, formatTime(now), id, workerID, expectedVersion, formatTime(now))
	if err != nil {
		return mapError("complete forest_asset action", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrLeaseLost
	}
	return nil
}

func (s *store) ReleaseAction(ctx context.Context, id, workerID domain.ID, expectedVersion int64, now time.Time) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_asset_actions SET status='claimed',worker_id=NULL,lease_expires_at=NULL,version=version+1,updated_at=?
		WHERE id=? AND worker_id=? AND status='claimed' AND version=?`, formatTime(now), id, workerID, expectedVersion)
	if err != nil {
		return mapError("release forest_asset action", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.ErrLeaseLost
	}
	return nil
}
