package sqlite

import (
	"context"
	"testing"
)

func TestRestartRestoresPatrolIndexState(t *testing.T) {
	db, _ := openTestDatabase(t)
	if err := db.RestorePatrolIndex(context.Background()); err == nil {
		t.Fatal("restart restored derived patrol state without rebuilding persisted index")
	}
}
