package sqlite

import (
	"context"
	"fmt"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

func (s *store) IdempotencyRecord(ctx context.Context, scope, key string) (repository.IdempotencyRecord, error) {
	var record repository.IdempotencyRecord
	var resourceID, created, expires string
	if err := s.queryer.QueryRowContext(ctx, `
		SELECT scope,idempotency_key,request_hash,resource_id,response_code,response_body,created_at,expires_at
		FROM idempotency_keys WHERE scope=? AND idempotency_key=?`, scope, key).Scan(&record.Scope, &record.Key, &record.RequestHash, &resourceID, &record.ResponseCode, &record.ResponseBody, &created, &expires); err != nil {
		return repository.IdempotencyRecord{}, mapError("get idempotency record", err)
	}
	record.ResourceID = domain.ID(resourceID)
	var err error
	if record.CreatedAt, err = parseTime(created); err != nil {
		return repository.IdempotencyRecord{}, fmt.Errorf("parse idempotency created_at: %w", err)
	}
	if record.ExpiresAt, err = parseTime(expires); err != nil {
		return repository.IdempotencyRecord{}, fmt.Errorf("parse idempotency expires_at: %w", err)
	}
	record.ResponseBody = append([]byte(nil), record.ResponseBody...)
	return record, nil
}

func (s *store) CreateIdempotencyRecord(ctx context.Context, record repository.IdempotencyRecord) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO idempotency_keys(scope,idempotency_key,request_hash,resource_id,response_code,response_body,created_at,expires_at)
		VALUES(?,?,?,?,?,?,?,?)`, record.Scope, record.Key, record.RequestHash, record.ResourceID, record.ResponseCode, record.ResponseBody, formatTime(record.CreatedAt), formatTime(record.ExpiresAt))
	return mapError("create idempotency record", err)
}
