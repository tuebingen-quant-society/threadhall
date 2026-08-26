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

func TestMessageStoreAdoptsOnlyExactVersionTwoSendIdempotency(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	t.Run("matching send returns original result", func(t *testing.T) {
		store, db := newVersionTwoMessageStore(t, now)
		result, err := store.Send(context.Background(), message.SendRecord{
			ConversationID: 1, AuthorID: 1, Body: "legacy", RenderedBody: "<p>legacy</p>\n",
			IdempotencyKey: "legacy-send", CreatedAt: now.Add(time.Hour),
		})
		if err != nil || result.Message.ID != 7 || result.Message.Body != "legacy" ||
			!result.Message.CreatedAt.Equal(now) || result.Event.Seq != 11 || result.Event.Type != "message.sent" {
			t.Fatalf("matching legacy Send = (%#v, %v)", result, err)
		}
		var mutations, messages, events int
		if err := db.QueryRow(`SELECT count(*),
			(SELECT count(*) FROM messages), (SELECT count(*) FROM events)
			FROM message_mutations`).Scan(&mutations, &messages, &events); err != nil {
			t.Fatalf("count adopted rows: %v", err)
		}
		if mutations != 1 || messages != 1 || events != 1 {
			t.Fatalf("adoption rows = mutations %d messages %d events %d, want 1/1/1", mutations, messages, events)
		}
	})

	t.Run("mismatched send conflicts", func(t *testing.T) {
		store, _ := newVersionTwoMessageStore(t, now)
		_, err := store.Send(context.Background(), message.SendRecord{
			ConversationID: 1, AuthorID: 1, Body: "different", RenderedBody: "<p>different</p>\n",
			IdempotencyKey: "legacy-send", CreatedAt: now,
		})
		if !errors.Is(err, message.ErrConflict) {
			t.Fatalf("mismatched legacy Send error = %v, want ErrConflict", err)
		}
	})

	for _, operation := range []struct {
		name string
		run  func(*MessageStore) error
	}{
		{name: "edit", run: func(store *MessageStore) error {
			_, err := store.Edit(context.Background(), message.EditRecord{
				MessageID: 7, AuthorID: 1, Body: "changed", RenderedBody: "<p>changed</p>\n",
				IdempotencyKey: "legacy-send", EditedAt: now.Add(time.Minute),
			})
			return err
		}},
		{name: "delete", run: func(store *MessageStore) error {
			_, err := store.Delete(context.Background(), message.DeleteRecord{
				MessageID: 7, AuthorID: 1, IdempotencyKey: "legacy-send", DeletedAt: now.Add(time.Minute),
			})
			return err
		}},
	} {
		t.Run(operation.name+" reuse conflicts", func(t *testing.T) {
			store, db := newVersionTwoMessageStore(t, now)
			if err := operation.run(store); !errors.Is(err, message.ErrConflict) {
				t.Fatalf("legacy key reused for %s error = %v, want ErrConflict", operation.name, err)
			}
			var body string
			var editedAt, deletedAt sql.NullInt64
			if err := db.QueryRow(`SELECT body, edited_at, deleted_at FROM messages WHERE id = 7`).
				Scan(&body, &editedAt, &deletedAt); err != nil {
				t.Fatalf("read preserved legacy message: %v", err)
			}
			if body != "legacy" || editedAt.Valid || deletedAt.Valid {
				t.Fatalf("legacy message changed: body=%q edited=%v deleted=%v", body, editedAt, deletedAt)
			}
		})
	}
}

func newVersionTwoMessageStore(t *testing.T, now time.Time) (*MessageStore, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "version-two-message.db")
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
	if _, err := legacy.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at)
			VALUES (1, 'author', 'hash', 1, ?);
		INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at)
			VALUES (1, 'channel', 'Legacy', 1, 'conversation', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (1, 1, ?);
		INSERT INTO messages(id, conversation_id, author_id, body, rendered_body, idempotency_key, created_at)
			VALUES (7, 1, 1, 'legacy', '<p>legacy</p>\n', 'legacy-send', ?);
		INSERT INTO events(seq, conversation_id, actor_id, kind, entity_id, payload, created_at)
			VALUES (11, 1, 1, 'message.sent', 7,
			'{"author_id":1,"body":"legacy","rendered_body":"<p>legacy</p>\\n","created_at":"2026-08-25T12:00:00Z"}', ?);`,
		now.Unix(), now.Unix(), now.Unix(), now.Unix(), now.Unix()); err != nil {
		t.Fatalf("seed version-two message: %v", err)
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
