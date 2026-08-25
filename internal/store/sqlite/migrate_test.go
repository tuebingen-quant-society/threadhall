package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAppliesCoreMigration(t *testing.T) {
	db := openTestDB(t)

	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != 2 {
		t.Fatalf("schema version = %d, want 2", version)
	}

	wantTables := []string{
		"users", "sessions", "invites", "conversations",
		"conversation_members", "conversation_mutations", "messages", "events",
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
	if version != 2 {
		t.Fatalf("schema version after rollback = %d, want 2", version)
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threadhall.db")
	db, err := Open(path, 1)
	if err != nil {
		t.Fatalf("initial Open: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 3"); err != nil {
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

func TestOpenUpgradesShippedCoreSchemaWithoutRewritingConversationRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	script, err := migrationFiles.ReadFile(migrations[0])
	if err != nil {
		t.Fatalf("read core migration: %v", err)
	}
	if err := applyMigration(context.Background(), legacy, 1, string(script)); err != nil {
		t.Fatalf("apply shipped migration: %v", err)
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC).Unix()
	if _, err := legacy.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES (1, 'admin', 'hash', 1, ?);
		INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at)
			VALUES (7, 'channel', 'Legacy', 1, 'legacy-key', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (7, 1, ?);`, now, now, now); err != nil {
		t.Fatalf("seed legacy conversation: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	upgraded, err := Open(path, 1)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	defer upgraded.Close()
	var version int
	var name string
	if err := upgraded.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read upgraded version: %v", err)
	}
	if err := upgraded.QueryRow("SELECT name FROM conversations WHERE id = 7").Scan(&name); err != nil {
		t.Fatalf("read preserved conversation: %v", err)
	}
	if version != 2 || name != "Legacy" {
		t.Fatalf("upgraded version/name = %d/%q, want 2/Legacy", version, name)
	}
	if _, err := upgraded.Exec(`INSERT INTO conversations(kind, name, created_by, idempotency_key, created_at)
		VALUES ('private', 'legacy', 1, 'collision', ?)`, now); err == nil {
		t.Fatal("upgraded database accepted case-insensitive channel-name collision")
	}
}
