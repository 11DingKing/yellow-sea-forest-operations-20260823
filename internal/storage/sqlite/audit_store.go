package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) AppendAudit(ctx context.Context, event domain.AuditEvent) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return fmt.Errorf("marshal audit metadata: %w", err)
	}
	_, err = s.queryer.ExecContext(ctx, `
		INSERT INTO audit_events(id,actor_id,action,object_type,object_id,result,request_id,metadata_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?)`, event.ID, event.ActorID, event.Action, event.ObjectType, event.ObjectID, event.Result, event.RequestID, string(metadata), formatTime(event.CreatedAt))
	return mapError("append audit event", err)
}

func (s *store) ListAudit(ctx context.Context, objectType string, objectID domain.ID, limit int) ([]domain.AuditEvent, error) {
	if limit < 1 || limit > 500 {
		return nil, domain.FieldError{Field: "limit", Message: "must be between 1 and 500"}
	}
	rows, err := s.queryer.QueryContext(ctx, `
		SELECT id,actor_id,action,object_type,object_id,result,request_id,metadata_json,created_at
		FROM audit_events WHERE object_type=? AND object_id=? ORDER BY created_at DESC,id DESC LIMIT ?`, objectType, objectID, limit)
	if err != nil {
		return nil, mapError("list audit events", err)
	}
	defer rows.Close()
	events := make([]domain.AuditEvent, 0)
	for rows.Next() {
		var event domain.AuditEvent
		var id, actorID, targetID, metadata, created string
		if err := rows.Scan(&id, &actorID, &event.Action, &event.ObjectType, &targetID, &event.Result, &event.RequestID, &metadata, &created); err != nil {
			return nil, err
		}
		event.ID, event.ActorID, event.ObjectID = domain.ID(id), domain.ID(actorID), domain.ID(targetID)
		if err := json.Unmarshal([]byte(metadata), &event.Metadata); err != nil {
			return nil, fmt.Errorf("decode audit metadata: %w", err)
		}
		event.CreatedAt, err = parseTime(created)
		if err != nil {
			return nil, err
		}
		events = append(events, event.Clone())
	}
	return events, rows.Err()
}
