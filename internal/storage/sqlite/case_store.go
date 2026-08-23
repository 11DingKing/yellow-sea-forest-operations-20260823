package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) CreateCase(ctx context.Context, item domain.ForestCase) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO forest_cases(id,forest_site_id,forest_parcel_id,reporter_id,owner_id,title,description,status,priority,version,submitted_at,updated_at,resolved_at,reopen_until)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, item.ID, item.ForestSiteID, item.ForestParcelID, item.ReporterID, nullableID(item.OwnerID), item.Title, item.Description, item.Status, item.Priority, item.Version, formatTime(item.SubmittedAt), formatTime(item.UpdatedAt), nullableTime(item.ResolvedAt), nullableTime(item.ReopenUntil))
	return mapError("create light case", err)
}

func scanCase(row interface{ Scan(...any) error }) (domain.ForestCase, error) {
	var item domain.ForestCase
	var id, forest_siteID, zoneID, reporterID, status, submitted, updated string
	var owner, resolved, reopen sql.NullString
	if err := row.Scan(&id, &forest_siteID, &zoneID, &reporterID, &owner, &item.Title, &item.Description, &status, &item.Priority, &item.Version, &submitted, &updated, &resolved, &reopen); err != nil {
		return domain.ForestCase{}, err
	}
	item.ID = domain.ID(id)
	item.ForestSiteID = domain.ID(forest_siteID)
	item.ForestParcelID = domain.ID(zoneID)
	item.ReporterID = domain.ID(reporterID)
	item.Status = domain.CaseStatus(status)
	if owner.Valid {
		value := domain.ID(owner.String)
		item.OwnerID = &value
	}
	var err error
	if item.SubmittedAt, err = parseTime(submitted); err != nil {
		return domain.ForestCase{}, fmt.Errorf("parse case submitted_at: %w", err)
	}
	if item.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.ForestCase{}, fmt.Errorf("parse case updated_at: %w", err)
	}
	if resolved.Valid {
		value, parseErr := parseTime(resolved.String)
		if parseErr != nil {
			return domain.ForestCase{}, parseErr
		}
		item.ResolvedAt = &value
	}
	if reopen.Valid {
		value, parseErr := parseTime(reopen.String)
		if parseErr != nil {
			return domain.ForestCase{}, parseErr
		}
		item.ReopenUntil = &value
	}
	return item, nil
}

const selectCase = `SELECT id,forest_site_id,forest_parcel_id,reporter_id,owner_id,title,description,status,priority,version,submitted_at,updated_at,resolved_at,reopen_until FROM forest_cases`

func (s *store) CaseByID(ctx context.Context, id domain.ID) (domain.ForestCase, error) {
	item, err := scanCase(s.queryer.QueryRowContext(ctx, selectCase+` WHERE id=?`, id))
	return item, mapError("get light case", err)
}

func (s *store) UpdateCase(ctx context.Context, item domain.ForestCase, expectedVersion int64) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE forest_cases SET owner_id=?,status=?,priority=?,version=?,updated_at=?,resolved_at=?,reopen_until=?
		WHERE id=? AND version=?`, nullableID(item.OwnerID), item.Status, item.Priority, item.Version, formatTime(item.UpdatedAt), nullableTime(item.ResolvedAt), nullableTime(item.ReopenUntil), item.ID, expectedVersion)
	if err != nil {
		return mapError("update light case", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return domain.VersionConflictError{Entity: "forest_case", Expected: expectedVersion, Actual: item.Version}
	}
	return nil
}

func (s *store) AppendCaseEvent(ctx context.Context, event domain.CaseEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return fmt.Errorf("marshal case event payload: %w", err)
	}
	_, err = s.queryer.ExecContext(ctx, `
		INSERT INTO case_events(id,case_id,actor_id,event_type,payload_json,request_id,created_at)
		VALUES(?,?,?,?,?,?,?)`, event.ID, event.CaseID, event.ActorID, event.Type, string(payload), event.RequestID, formatTime(event.CreatedAt))
	return mapError("append case event", err)
}

func (s *store) ListCaseEvents(ctx context.Context, caseID domain.ID) ([]domain.CaseEvent, error) {
	rows, err := s.queryer.QueryContext(ctx, `
		SELECT id,case_id,actor_id,event_type,payload_json,request_id,created_at
		FROM case_events WHERE case_id=? ORDER BY created_at,id`, caseID)
	if err != nil {
		return nil, mapError("list case events", err)
	}
	defer rows.Close()
	events := make([]domain.CaseEvent, 0)
	for rows.Next() {
		var event domain.CaseEvent
		var id, eventCaseID, actorID, payload, created string
		if err := rows.Scan(&id, &eventCaseID, &actorID, &event.Type, &payload, &event.RequestID, &created); err != nil {
			return nil, err
		}
		event.ID, event.CaseID, event.ActorID = domain.ID(id), domain.ID(eventCaseID), domain.ID(actorID)
		if err := json.Unmarshal([]byte(payload), &event.Payload); err != nil {
			return nil, fmt.Errorf("decode case event payload: %w", err)
		}
		event.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		events = append(events, event.Clone())
	}
	return events, rows.Err()
}

func caseFilterSQL(filter domain.CaseFilter) (string, []any) {
	clauses := []string{"1=1"}
	args := make([]any, 0)
	if len(filter.Statuses) > 0 {
		placeholders := make([]string, len(filter.Statuses))
		for index, status := range filter.Statuses {
			placeholders[index] = "?"
			args = append(args, status)
		}
		clauses = append(clauses, `status IN (`+strings.Join(placeholders, ",")+`)`)
	}
	if filter.ForestSiteID != nil {
		clauses, args = append(clauses, "forest_site_id=?"), append(args, *filter.ForestSiteID)
	}
	if filter.ZoneID != nil {
		clauses, args = append(clauses, "forest_parcel_id=?"), append(args, *filter.ZoneID)
	}
	if filter.OwnerID != nil {
		clauses, args = append(clauses, "owner_id=?"), append(args, *filter.OwnerID)
	}
	if filter.CreatedFrom != nil {
		clauses, args = append(clauses, "submitted_at>=?"), append(args, formatTime(*filter.CreatedFrom))
	}
	if filter.CreatedTo != nil {
		clauses, args = append(clauses, "submitted_at<?"), append(args, formatTime(*filter.CreatedTo))
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		clauses, args = append(clauses, "(title LIKE ? ESCAPE '\\' OR description LIKE ? ESCAPE '\\')"), append(args, "%"+search+"%", "%"+search+"%")
	}
	return strings.Join(clauses, " AND "), args
}

func (s *store) ListCases(ctx context.Context, filter domain.CaseFilter, page domain.PageRequest) (domain.Page[domain.ForestCase], error) {
	page = page.Normalize(map[string]bool{"submitted_at": true, "updated_at": true, "priority": true}, "submitted_at")
	where, args := caseFilterSQL(filter.Clone())
	var total int
	if err := s.queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM forest_cases WHERE `+where, args...).Scan(&total); err != nil {
		return domain.Page[domain.ForestCase]{}, mapError("count light cases", err)
	}
	direction := "ASC"
	if page.Desc {
		direction = "DESC"
	}
	queryArgs := append(append([]any(nil), args...), page.PageSize, page.Offset())
	rows, err := s.queryer.QueryContext(ctx, selectCase+` WHERE `+where+` ORDER BY `+page.Sort+` `+direction+`,id `+direction+` LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return domain.Page[domain.ForestCase]{}, mapError("list light cases", err)
	}
	defer rows.Close()
	items := make([]domain.ForestCase, 0, page.PageSize)
	for rows.Next() {
		item, scanErr := scanCase(rows)
		if scanErr != nil {
			return domain.Page[domain.ForestCase]{}, scanErr
		}
		items = append(items, item.Clone())
	}
	return domain.Page[domain.ForestCase]{Items: items, Page: page.Page, PageSize: page.PageSize, Total: total}, rows.Err()
}

func residentScopeClause() string { return "all_forest_parcels" }
