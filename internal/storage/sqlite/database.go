package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/domain"
	"github.com/11DingKing/yellow-sea-forest-operations-20260823/internal/repository"
)

type Database struct {
	db *sql.DB
}

func Open(ctx context.Context, dataSource string) (*Database, error) {
	db, err := sql.Open("sqlite", dataSource)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(30 * time.Minute)
	database := &Database{db: db}
	if err := database.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := database.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return database, nil
}

func (d *Database) Close() error { return d.db.Close() }

func (d *Database) Ping(ctx context.Context) error {
	if err := d.db.PingContext(ctx); err != nil {
		return domain.WrapUnavailable("ping database", err)
	}
	return nil
}

func (d *Database) SchemaVersion(ctx context.Context) (int, error) {
	var version int
	if err := d.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

func (d *Database) migrate(ctx context.Context) error {
	connection, err := d.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	var version int
	if err := connection.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("database schema %d is newer than supported %d", version, currentSchemaVersion)
	}
	if version == 0 {
		if _, err := connection.ExecContext(ctx, migrationV1); err != nil {
			return fmt.Errorf("apply migration v1: %w", err)
		}
		if _, err := connection.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", currentSchemaVersion)); err != nil {
			return fmt.Errorf("record migration version: %w", err)
		}
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	committed = true
	return nil
}

func (d *Database) Store() repository.Store { return &store{queryer: d.db} }

func (d *Database) RestorePatrolIndex(ctx context.Context) error { return nil }

func (d *Database) WithinTx(ctx context.Context, operation func(repository.Store) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return domain.WrapUnavailable("begin transaction", err)
	}
	finished := false
	defer func() {
		if !finished {
			_ = tx.Rollback()
		}
	}()
	if err := operation(&store{queryer: tx}); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		finished = true
		return err
	}
	if err := tx.Commit(); err != nil {
		return domain.WrapUnavailable("commit transaction", err)
	}
	finished = true
	return nil
}

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type store struct {
	queryer queryer
}

func mapError(operation string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "constraint failed") {
		return fmt.Errorf("%s: %w: %v", operation, domain.ErrConflict, err)
	}
	if strings.Contains(message, "database is locked") || strings.Contains(message, "database is busy") {
		return domain.WrapUnavailable(operation, err)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func formatTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func nullableID(value *domain.ID) any {
	if value == nil {
		return nil
	}
	return value.String()
}
