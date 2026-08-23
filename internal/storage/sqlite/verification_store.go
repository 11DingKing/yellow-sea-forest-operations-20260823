package sqlite

import (
	"context"
	"database/sql"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) CreateInspection(ctx context.Context, round domain.InspectionRound) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO inspection_rounds(id,case_id,plan_id,inspector_id,status,observed_max_lux,resident_agreed,notes,version,created_at,completed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, round.ID, round.CaseID, round.PlanID, round.InspectorID, round.Status, round.ObservedMaxLux, round.ResidentAgreed, round.Notes, round.Version, formatTime(round.CreatedAt), nullableTime(round.CompletedAt))
	return mapError("create verification round", err)
}

func scanInspection(row interface{ Scan(...any) error }) (domain.InspectionRound, error) {
	var round domain.InspectionRound
	var id, caseID, planID, inspectorID, status, created string
	var completed sql.NullString
	if err := row.Scan(&id, &caseID, &planID, &inspectorID, &status, &round.ObservedMaxLux, &round.ResidentAgreed, &round.Notes, &round.Version, &created, &completed); err != nil {
		return domain.InspectionRound{}, err
	}
	round.ID, round.CaseID, round.PlanID, round.InspectorID = domain.ID(id), domain.ID(caseID), domain.ID(planID), domain.ID(inspectorID)
	round.Status = domain.InspectionStatus(status)
	var err error
	if round.CreatedAt, err = parseTime(created); err != nil {
		return domain.InspectionRound{}, err
	}
	if completed.Valid {
		value, parseErr := parseTime(completed.String)
		if parseErr != nil {
			return domain.InspectionRound{}, parseErr
		}
		round.CompletedAt = &value
	}
	return round, nil
}

const selectInspection = `SELECT id,case_id,plan_id,inspector_id,status,observed_max_lux,resident_agreed,notes,version,created_at,completed_at FROM inspection_rounds`

func (s *store) InspectionByID(ctx context.Context, id domain.ID) (domain.InspectionRound, error) {
	round, err := scanInspection(s.queryer.QueryRowContext(ctx, selectInspection+` WHERE id=?`, id))
	return round, mapError("get verification round", err)
}

func (s *store) OpenInspectionForCase(ctx context.Context, caseID domain.ID) (domain.InspectionRound, error) {
	round, err := scanInspection(s.queryer.QueryRowContext(ctx, selectInspection+` WHERE case_id=? AND status='open'`, caseID))
	return round, mapError("get open verification round", err)
}

func (s *store) UpdateInspection(ctx context.Context, round domain.InspectionRound, expectedVersion int64) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE inspection_rounds SET status=?,observed_max_lux=?,resident_agreed=?,notes=?,version=?,completed_at=? WHERE id=? AND version=?`, round.Status, round.ObservedMaxLux, round.ResidentAgreed, round.Notes, round.Version, nullableTime(round.CompletedAt), round.ID, expectedVersion)
	if err != nil {
		return mapError("update verification round", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.VersionConflictError{Entity: "inspection_round", Expected: expectedVersion, Actual: round.Version}
	}
	return nil
}
