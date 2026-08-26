package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenBackfillsActiveAgentsIntoExistingPublicChannels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-ten.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open version-ten database: %v", err)
	}
	for index := 0; index < len(migrations)-1; index++ {
		script, readErr := migrationFiles.ReadFile(migrations[index])
		if readErr != nil {
			t.Fatalf("read migration %d: %v", index+1, readErr)
		}
		if err := applyMigration(context.Background(), legacy, index+1, string(script)); err != nil {
			t.Fatalf("apply migration %d: %v", index+1, err)
		}
	}
	now := time.Date(2026, 8, 26, 18, 0, 0, 0, time.UTC).Unix()
	token := sha256.Sum256([]byte("worker token"))
	if _, err := legacy.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at)
			VALUES (1, 'admin', 'hash', 1, ?);
		INSERT INTO users(id, username, password_hash, is_admin, created_at, principal_kind)
			VALUES (2, 'codex', '!', 0, ?, 'agent');
		INSERT INTO agents(user_id, token_hash, created_by, created_at) VALUES (2, ?, 1, ?);
		INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at) VALUES
			(1, 'channel', 'public', 1, 'public', ?),
			(2, 'private', 'private', 1, 'private', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES
			(1, 1, ?), (2, 1, ?);`, now, now, token[:], now, now, now, now, now); err != nil {
		t.Fatalf("seed version-ten database: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close version-ten database: %v", err)
	}

	upgraded, err := Open(path, 1)
	if err != nil {
		t.Fatalf("upgrade version-ten database: %v", err)
	}
	defer upgraded.Close()
	assertAgentAccess(t, upgraded, 1, 1)
	assertAgentAccess(t, upgraded, 2, 0)
}
