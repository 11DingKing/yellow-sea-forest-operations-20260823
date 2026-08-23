package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) CreateForestSurvey(ctx context.Context, forest_survey domain.ForestSurvey) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO surveys(id,case_id,inspector_id,status,started_at,published_at,summary_lux,version,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, forest_survey.ID, forest_survey.CaseID, forest_survey.InspectorID, forest_survey.Status, formatTime(forest_survey.StartedAt), nullableTime(forest_survey.PublishedAt), forest_survey.SummaryLux, forest_survey.Version, formatTime(forest_survey.CreatedAt), formatTime(forest_survey.UpdatedAt))
	return mapError("create forest_survey", err)
}

func scanForestSurvey(row interface{ Scan(...any) error }) (domain.ForestSurvey, error) {
	var forest_survey domain.ForestSurvey
	var id, caseID, inspectorID, status, started, created, updated string
	var published sql.NullString
	if err := row.Scan(&id, &caseID, &inspectorID, &status, &started, &published, &forest_survey.SummaryLux, &forest_survey.Version, &created, &updated); err != nil {
		return domain.ForestSurvey{}, err
	}
	forest_survey.ID = domain.ID(id)
	forest_survey.CaseID = domain.ID(caseID)
	forest_survey.InspectorID = domain.ID(inspectorID)
	forest_survey.Status = domain.ForestSurveyStatus(status)
	var err error
	if forest_survey.StartedAt, err = parseTime(started); err != nil {
		return domain.ForestSurvey{}, err
	}
	if forest_survey.CreatedAt, err = parseTime(created); err != nil {
		return domain.ForestSurvey{}, err
	}
	if forest_survey.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ForestSurvey{}, err
	}
	if published.Valid {
		value, parseErr := parseTime(published.String)
		if parseErr != nil {
			return domain.ForestSurvey{}, parseErr
		}
		forest_survey.PublishedAt = &value
	}
	return forest_survey, nil
}

const selectForestSurvey = `SELECT id,case_id,inspector_id,status,started_at,published_at,summary_lux,version,created_at,updated_at FROM surveys`

func (s *store) ForestSurveyByID(ctx context.Context, id domain.ID) (domain.ForestSurvey, error) {
	forest_survey, err := scanForestSurvey(s.queryer.QueryRowContext(ctx, selectForestSurvey+` WHERE id=?`, id))
	return forest_survey, mapError("get forest_survey", err)
}

func (s *store) OpenForestSurveyForCase(ctx context.Context, caseID domain.ID) (domain.ForestSurvey, error) {
	forest_survey, err := scanForestSurvey(s.queryer.QueryRowContext(ctx, selectForestSurvey+` WHERE case_id=? AND status='draft'`, caseID))
	return forest_survey, mapError("get open forest_survey", err)
}

func (s *store) UpdateForestSurvey(ctx context.Context, forest_survey domain.ForestSurvey, expectedVersion int64) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE surveys SET status=?,published_at=?,summary_lux=?,version=?,updated_at=?
		WHERE id=? AND version=?`, forest_survey.Status, nullableTime(forest_survey.PublishedAt), forest_survey.SummaryLux, forest_survey.Version, formatTime(forest_survey.UpdatedAt), forest_survey.ID, expectedVersion)
	if err != nil {
		return mapError("update forest_survey", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.VersionConflictError{Entity: "forest_survey", Expected: expectedVersion, Actual: forest_survey.Version}
	}
	return nil
}

func (s *store) AddReading(ctx context.Context, reading domain.ForestSurveyReading) error {
	var status string
	if err := s.queryer.QueryRowContext(ctx, `SELECT status FROM surveys WHERE id=?`, reading.ForestSurveyID).Scan(&status); err != nil {
		return mapError("check forest_survey before reading", err)
	}
	if status != string(domain.ForestSurveyDraft) {
		return domain.TransitionError{Entity: "forest_survey", From: status, To: "add_reading"}
	}
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO forest_survey_readings(id,forest_survey_id,position,lux,measured_at,sequence)
		VALUES(?,?,?,?,?,?)`, reading.ID, reading.ForestSurveyID, reading.Position, reading.Lux, formatTime(reading.MeasuredAt), reading.Sequence)
	return mapError("add forest_survey reading", err)
}

func (s *store) ListReadings(ctx context.Context, forest_surveyID domain.ID) ([]domain.ForestSurveyReading, error) {
	rows, err := s.queryer.QueryContext(ctx, `
		SELECT id,forest_survey_id,position,lux,measured_at,sequence
		FROM forest_survey_readings WHERE forest_survey_id=? ORDER BY sequence,id`, forest_surveyID)
	if err != nil {
		return nil, mapError("list forest_survey readings", err)
	}
	defer rows.Close()
	readings := make([]domain.ForestSurveyReading, 0)
	for rows.Next() {
		var reading domain.ForestSurveyReading
		var id, ownerID, measured string
		if err := rows.Scan(&id, &ownerID, &reading.Position, &reading.Lux, &measured, &reading.Sequence); err != nil {
			return nil, fmt.Errorf("scan forest_survey reading: %w", err)
		}
		reading.ID, reading.ForestSurveyID = domain.ID(id), domain.ID(ownerID)
		reading.MeasuredAt, err = parseTime(measured)
		if err != nil {
			return nil, err
		}
		readings = append(readings, reading)
	}
	return domain.CloneReadings(readings), rows.Err()
}
