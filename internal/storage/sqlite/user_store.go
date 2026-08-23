package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
)

func (s *store) CreateUser(ctx context.Context, user domain.User) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO users(id,email,display_name,password_hash,role,active,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?)`, user.ID, user.Email, user.DisplayName, user.PasswordHash, user.Role, user.Active, formatTime(user.CreatedAt), formatTime(user.UpdatedAt))
	return mapError("create user", err)
}

func scanUser(row interface{ Scan(...any) error }) (domain.User, error) {
	var user domain.User
	var id string
	var role string
	var active bool
	var created, updated string
	if err := row.Scan(&id, &user.Email, &user.DisplayName, &user.PasswordHash, &role, &active, &created, &updated); err != nil {
		return domain.User{}, err
	}
	user.ID = domain.ID(id)
	user.Role = domain.Role(role)
	user.Active = active
	var err error
	if user.CreatedAt, err = parseTime(created); err != nil {
		return domain.User{}, fmt.Errorf("parse user created_at: %w", err)
	}
	if user.UpdatedAt, err = parseTime(updated); err != nil {
		return domain.User{}, fmt.Errorf("parse user updated_at: %w", err)
	}
	user.PasswordHash = append([]byte(nil), user.PasswordHash...)
	return user, nil
}

const selectUser = `SELECT id,email,display_name,password_hash,role,active,created_at,updated_at FROM users`

func (s *store) UserByID(ctx context.Context, id domain.ID) (domain.User, error) {
	user, err := scanUser(s.queryer.QueryRowContext(ctx, selectUser+` WHERE id=?`, id))
	return user, mapError("get user by id", err)
}

func (s *store) UserByEmail(ctx context.Context, email string) (domain.User, error) {
	user, err := scanUser(s.queryer.QueryRowContext(ctx, selectUser+` WHERE email=? COLLATE NOCASE`, email))
	return user, mapError("get user by email", err)
}

func (s *store) CreateSession(ctx context.Context, session domain.Session) error {
	_, err := s.queryer.ExecContext(ctx, `
		INSERT INTO sessions(id,user_id,token_hash,created_at,expires_at,revoked_at,user_agent,ip)
		VALUES(?,?,?,?,?,?,?,?)`, session.ID, session.UserID, session.TokenHash, formatTime(session.CreatedAt), formatTime(session.ExpiresAt), nullableTime(session.RevokedAt), session.UserAgent, session.IP)
	return mapError("create session", err)
}

func scanSession(row interface{ Scan(...any) error }) (domain.Session, error) {
	var session domain.Session
	var id, userID, created, expires string
	var revoked sql.NullString
	if err := row.Scan(&id, &userID, &session.TokenHash, &created, &expires, &revoked, &session.UserAgent, &session.IP); err != nil {
		return domain.Session{}, err
	}
	session.ID = domain.ID(id)
	session.UserID = domain.ID(userID)
	var err error
	if session.CreatedAt, err = parseTime(created); err != nil {
		return domain.Session{}, fmt.Errorf("parse session created_at: %w", err)
	}
	if session.ExpiresAt, err = parseTime(expires); err != nil {
		return domain.Session{}, fmt.Errorf("parse session expires_at: %w", err)
	}
	if revoked.Valid {
		value, parseErr := parseTime(revoked.String)
		if parseErr != nil {
			return domain.Session{}, fmt.Errorf("parse session revoked_at: %w", parseErr)
		}
		session.RevokedAt = &value
	}
	session.TokenHash = append([]byte(nil), session.TokenHash...)
	return session, nil
}

func (s *store) SessionByTokenHash(ctx context.Context, hash []byte) (domain.Session, error) {
	row := s.queryer.QueryRowContext(ctx, `
		SELECT id,user_id,token_hash,created_at,expires_at,revoked_at,user_agent,ip
		FROM sessions WHERE token_hash=?`, hash)
	session, err := scanSession(row)
	return session, mapError("get session by token", err)
}

func (s *store) RevokeSession(ctx context.Context, id domain.ID, now time.Time) error {
	result, err := s.queryer.ExecContext(ctx, `
		UPDATE sessions SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, formatTime(now), id)
	if err != nil {
		return mapError("revoke session", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke session rows affected: %w", err)
	}
	if rows == 0 {
		var existing string
		if lookupErr := s.queryer.QueryRowContext(ctx, `SELECT id FROM sessions WHERE id=?`, id).Scan(&existing); lookupErr != nil {
			return mapError("find session for revoke", lookupErr)
		}
	}
	return nil
}

func (s *store) DeleteExpiredSessions(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 1000 {
		return 0, domain.FieldError{Field: "limit", Message: "must be between 1 and 1000"}
	}
	result, err := s.queryer.ExecContext(ctx, `
		DELETE FROM sessions WHERE id IN (
			SELECT id FROM sessions WHERE expires_at<=? OR revoked_at IS NOT NULL ORDER BY expires_at LIMIT ?
		)`, formatTime(now), limit)
	if err != nil {
		return 0, mapError("delete expired sessions", err)
	}
	rows, err := result.RowsAffected()
	return int(rows), err
}
