package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOpenRenamesV1ChannelCollisionsDeterministicallyBeforeUniqueIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-collisions.db")
	legacy := openLegacyCoreDatabase(t, path)
	names := []string{
		"General", "general", "GENERAL", "general~2", "Staff", "STAFF",
		strings.Repeat("A", 80), strings.Repeat("a", 80),
	}
	for index, name := range names {
		kind := "channel"
		if index == 1 || index == 3 {
			kind = "private"
		}
		id := index + 1
		if _, err := legacy.Exec(`INSERT INTO conversations(
			id, kind, name, created_by, idempotency_key, created_at) VALUES (?, ?, ?, 1, ?, 100)`,
			id, kind, name, "legacy-key-"+string(rune('a'+index))); err != nil {
			t.Fatalf("seed conversation %d: %v", id, err)
		}
		if _, err := legacy.Exec(`INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (?, 1, 100)`, id); err != nil {
			t.Fatalf("seed membership %d: %v", id, err)
		}
	}
	if _, err := legacy.Exec(`
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (2, 2, 100);
		INSERT INTO messages(id, conversation_id, author_id, body, rendered_body, idempotency_key, created_at)
			VALUES (21, 2, 1, 'preserve me', 'preserve me', 'message-key', 100);
		INSERT INTO events(seq, conversation_id, actor_id, kind, entity_id, payload, created_at)
			VALUES (31, 2, 1, 'legacy.event', 21, '{"preserve":true}', 100);`); err != nil {
		t.Fatalf("seed related data: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	upgraded, err := Open(path, 2)
	if err != nil {
		t.Fatalf("upgrade colliding legacy database: %v", err)
	}
	defer upgraded.Close()
	wantNames := []string{
		"General", "general~2~2", "GENERAL~3", "general~2", "Staff", "STAFF~6",
		strings.Repeat("A", 80), strings.Repeat("a", 78) + "~8",
	}
	rows, err := upgraded.Query(`SELECT id, name FROM conversations WHERE kind IN ('channel', 'private') ORDER BY id`)
	if err != nil {
		t.Fatalf("read upgraded names: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for index := 0; rows.Next(); index++ {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan upgraded name: %v", err)
		}
		if id != index+1 || name != wantNames[index] {
			t.Fatalf("conversation %d name = %q, want %q", id, name, wantNames[index])
		}
		if len(name) > 80 || !utf8.ValidString(name) {
			t.Fatalf("conversation %d has invalid bounded name %q", id, name)
		}
		key := asciiNoCaseTestKey(name)
		if seen[key] {
			t.Fatalf("case-insensitive duplicate after upgrade: %q", name)
		}
		seen[key] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate upgraded names: %v", err)
	}
	var kind, body, payload string
	var memberships int
	if err := upgraded.QueryRow(`SELECT kind FROM conversations WHERE id = 2`).Scan(&kind); err != nil {
		t.Fatalf("read preserved kind: %v", err)
	}
	if err := upgraded.QueryRow(`SELECT count(*) FROM conversation_members`).Scan(&memberships); err != nil {
		t.Fatalf("count preserved memberships: %v", err)
	}
	if err := upgraded.QueryRow(`SELECT body FROM messages WHERE id = 21`).Scan(&body); err != nil {
		t.Fatalf("read preserved message: %v", err)
	}
	if err := upgraded.QueryRow(`SELECT payload FROM events WHERE seq = 31`).Scan(&payload); err != nil {
		t.Fatalf("read preserved event: %v", err)
	}
	if kind != "private" || memberships != 9 || body != "preserve me" || payload != `{"preserve":true}` {
		t.Fatalf("preserved data = kind %q memberships %d body %q payload %q", kind, memberships, body, payload)
	}
}

func openLegacyCoreDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	script, err := migrationFiles.ReadFile(migrations[0])
	if err != nil {
		t.Fatalf("read core migration: %v", err)
	}
	if err := applyMigration(context.Background(), db, 1, string(script)); err != nil {
		t.Fatalf("apply core migration: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES
		(1, 'admin', 'hash', 1, 100), (2, 'member', 'hash', 0, 100)`); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	return db
}

func asciiNoCaseTestKey(value string) string {
	bytes := []byte(value)
	for index, char := range bytes {
		if char >= 'A' && char <= 'Z' {
			bytes[index] = char + ('a' - 'A')
		}
	}
	return string(bytes)
}
