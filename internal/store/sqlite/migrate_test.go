package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAppliesCoreMigration(t *testing.T) {
	db := openTestDB(t)

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version = %d, want 1", version)
	}

	wantTables := []string{
		"users", "sessions", "invites", "conversations",
		"conversation_members", "messages", "events",
	}
	for _, table := range wantTables {
		var count int
		err := db.QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?",
			table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("look up table %q: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %q count = %d, want 1", table, count)
		}
	}
}

func TestApplyMigrationRollsBackScriptAndVersion(t *testing.T) {
	db := openTestDB(t)
	err := applyMigration(context.Background(), db, 2, `
		CREATE TABLE must_rollback (id INTEGER PRIMARY KEY);
		THIS IS NOT SQL;
	`)
	if err == nil {
		t.Fatal("applyMigration() error = nil, want invalid-SQL error")
	}

	var tableCount int
	if err := db.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'must_rollback'",
	).Scan(&tableCount); err != nil {
		t.Fatalf("look up rolled-back table: %v", err)
	}
	if tableCount != 0 {
		t.Fatalf("rolled-back table count = %d, want 0", tableCount)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 1 {
		t.Fatalf("schema version after rollback = %d, want 1", version)
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threadhall.db")
	db, err := Open(path, 1)
	if err != nil {
		t.Fatalf("initial Open: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatalf("advance schema version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close initial database: %v", err)
	}

	db, err = Open(path, 1)
	if err == nil {
		_ = db.Close()
		t.Fatal("Open() error = nil, want newer-schema refusal")
	}
}
