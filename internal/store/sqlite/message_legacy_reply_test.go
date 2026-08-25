package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestMessageStoreTreatsVersionTwoRepliesAsInaccessibleLegacyRows(t *testing.T) {
	store, db := newVersionTwoReplyStore(t)
	page, err := store.History(context.Background(), message.History{
		ConversationID: 1, UserID: 1, Limit: 50,
	})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != 1 || page.Messages[0].Body != "root" {
		t.Fatalf("root history = (%#v, %v)", page, err)
	}
	now := time.Date(2026, 8, 26, 0, 30, 0, 0, time.UTC)
	if _, err := store.Edit(context.Background(), message.EditRecord{
		MessageID: 2, AuthorID: 1, Body: "changed reply", RenderedBody: "<p>changed reply</p>\n",
		IdempotencyKey: "edit-reply", EditedAt: now,
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("reply Edit error = %v, want ErrNotFound", err)
	}
	if _, err := store.Delete(context.Background(), message.DeleteRecord{
		MessageID: 2, AuthorID: 1, IdempotencyKey: "delete-reply", DeletedAt: now,
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("reply Delete error = %v, want ErrNotFound", err)
	}
	var raw, rendered string
	var editedAt, deletedAt sql.NullInt64
	var events, mutations int
	if err := db.QueryRow(`SELECT body, rendered_body, edited_at, deleted_at,
		(SELECT count(*) FROM events), (SELECT count(*) FROM message_mutations)
		FROM messages WHERE id = 2`).Scan(&raw, &rendered, &editedAt, &deletedAt, &events, &mutations); err != nil {
		t.Fatalf("read legacy reply: %v", err)
	}
	if raw != "reply" || rendered != "<p>reply</p>" || editedAt.Valid || deletedAt.Valid || events != 0 || mutations != 0 {
		t.Fatalf("legacy reply changed: raw=%q rendered=%q edited=%v deleted=%v events=%d mutations=%d",
			raw, rendered, editedAt, deletedAt, events, mutations)
	}
}

func newVersionTwoReplyStore(t *testing.T) (*MessageStore, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "version-two-replies.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open version-two database: %v", err)
	}
	for index := 0; index < 2; index++ {
		script, err := migrationFiles.ReadFile(migrations[index])
		if err != nil {
			t.Fatalf("read migration %d: %v", index+1, err)
		}
		if err := applyMigration(context.Background(), legacy, index+1, string(script)); err != nil {
			t.Fatalf("apply migration %d: %v", index+1, err)
		}
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC).Unix()
	if _, err := legacy.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES (1, 'author', 'hash', 1, ?);
		INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at)
			VALUES (1, 'private', 'legacy', 1, 'conversation', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (1, 1, ?);
		INSERT INTO messages(id, conversation_id, author_id, reply_to_id, body, rendered_body, idempotency_key, created_at) VALUES
			(1, 1, 1, NULL, 'root', '<p>root</p>', 'root', ?),
			(2, 1, 1, 1, 'reply', '<p>reply</p>', 'reply', ?);`, now, now, now, now, now); err != nil {
		t.Fatalf("seed version-two replies: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close version-two database: %v", err)
	}
	db, err := Open(path, 2)
	if err != nil {
		t.Fatalf("upgrade version-two database: %v", err)
	}
	writer, err := NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = db.Close() })
	return NewMessageStore(db, writer), db
}
