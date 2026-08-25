package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func TestMessageHTTPRealSQLiteEnforcesMembershipOwnershipAndIdempotency(t *testing.T) {
	service, db := newHTTPMessageService(t)
	csrf := tokenString(0x57)
	author := testMessageHandler(&fakeAuthAPI{user: auth.User{ID: 1, Username: "author"}}, service)
	member := testMessageHandler(&fakeAuthAPI{user: auth.User{ID: 2, Username: "member"}}, service)
	outsider := testMessageHandler(&fakeAuthAPI{user: auth.User{ID: 3, Username: "outsider"}}, service)

	sent := messageJSONMutation(t, author, http.MethodPost, "/api/v1/conversations/1/messages", map[string]any{
		"body": "**safe** <script>pwn()</script>", "idempotency_key": "send",
	}, csrf, true)
	if sent.Code != http.StatusCreated {
		t.Fatalf("send status = %d; body=%s", sent.Code, sent.Body.String())
	}
	var first message.Result
	if err := json.NewDecoder(sent.Body).Decode(&first); err != nil {
		t.Fatalf("decode send: %v", err)
	}
	if first.Message.RenderedBody != "<p><strong>safe</strong> pwn()</p>\n" {
		t.Fatalf("rendered body = %q", first.Message.RenderedBody)
	}

	memberHistory := messageRead(t, member, "/api/v1/conversations/1/messages")
	if memberHistory.Code != http.StatusOK {
		t.Fatalf("member history status = %d; body=%s", memberHistory.Code, memberHistory.Body.String())
	}
	missing := messageRead(t, outsider, "/api/v1/conversations/1/messages")
	unknown := messageRead(t, outsider, "/api/v1/conversations/999/messages")
	if missing.Code != http.StatusNotFound || missing.Body.String() != unknown.Body.String() {
		t.Fatalf("inaccessible/missing = %d %q / %d %q", missing.Code, missing.Body.String(), unknown.Code, unknown.Body.String())
	}

	unauthorizedEdit := messageJSONMutation(t, member, http.MethodPatch, "/api/v1/messages/"+idString(first.Message.ID),
		map[string]any{"body": "stolen", "idempotency_key": "member-edit"}, csrf, true)
	if unauthorizedEdit.Code != http.StatusNotFound {
		t.Fatalf("non-author edit status = %d; body=%s", unauthorizedEdit.Code, unauthorizedEdit.Body.String())
	}
	edited := messageJSONMutation(t, author, http.MethodPatch, "/api/v1/messages/"+idString(first.Message.ID),
		map[string]any{"body": "changed", "idempotency_key": "edit"}, csrf, true)
	if edited.Code != http.StatusOK {
		t.Fatalf("author edit status = %d; body=%s", edited.Code, edited.Body.String())
	}
	replayedSend := messageJSONMutation(t, author, http.MethodPost, "/api/v1/conversations/1/messages", map[string]any{
		"body": "**safe** <script>pwn()</script>", "idempotency_key": "send",
	}, csrf, true)
	var replayed message.Result
	if replayedSend.Code != http.StatusCreated || json.NewDecoder(replayedSend.Body).Decode(&replayed) != nil ||
		replayed.Message.Body != first.Message.Body || replayed.Event.Seq != first.Event.Seq {
		t.Fatalf("replayed send = status %d result %#v", replayedSend.Code, replayed)
	}
	conflict := messageJSONMutation(t, author, http.MethodPost, "/api/v1/conversations/1/messages", map[string]any{
		"body": "different", "idempotency_key": "send",
	}, csrf, true)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict status = %d; body=%s", conflict.Code, conflict.Body.String())
	}
	deleted := messageJSONMutation(t, author, http.MethodDelete, "/api/v1/messages/"+idString(first.Message.ID),
		map[string]any{"idempotency_key": "delete"}, csrf, true)
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status = %d; body=%s", deleted.Code, deleted.Body.String())
	}
	var raw, rendered string
	if err := db.QueryRow("SELECT body, rendered_body FROM messages WHERE id = ?", first.Message.ID).
		Scan(&raw, &rendered); err != nil || raw != "" || rendered != "" {
		t.Fatalf("stored tombstone = %q/%q, %v", raw, rendered, err)
	}
}

func TestMessageHTTPVersionTwoRepliesStayOutsideRootHistoryAndMutations(t *testing.T) {
	service, db := newHTTPVersionTwoReplyService(t)
	handler := testMessageHandler(&fakeAuthAPI{user: auth.User{ID: 1, Username: "author"}}, service)
	csrf := tokenString(0x58)
	history := messageRead(t, handler, "/api/v1/conversations/1/messages")
	var page message.Page
	if history.Code != http.StatusOK || json.NewDecoder(history.Body).Decode(&page) != nil ||
		len(page.Messages) != 1 || page.Messages[0].ID != 1 {
		t.Fatalf("root history = status %d page %#v; body=%s", history.Code, page, history.Body.String())
	}
	edit := messageJSONMutation(t, handler, http.MethodPatch, "/api/v1/messages/2",
		map[string]any{"body": "changed reply", "idempotency_key": "edit-reply"}, csrf, true)
	deletion := messageJSONMutation(t, handler, http.MethodDelete, "/api/v1/messages/2",
		map[string]any{"idempotency_key": "delete-reply"}, csrf, true)
	if edit.Code != http.StatusNotFound || edit.Body.String() != deletion.Body.String() {
		t.Fatalf("reply edit/delete = %d %q / %d %q", edit.Code, edit.Body.String(), deletion.Code, deletion.Body.String())
	}
	var raw, rendered string
	var editedAt, deletedAt sql.NullInt64
	var events, mutations int
	if err := db.QueryRow(`SELECT body, rendered_body, edited_at, deleted_at,
		(SELECT count(*) FROM events), (SELECT count(*) FROM message_mutations)
		FROM messages WHERE id = 2`).Scan(&raw, &rendered, &editedAt, &deletedAt, &events, &mutations); err != nil {
		t.Fatalf("read version-two reply: %v", err)
	}
	if raw != "reply" || rendered != "<p>reply</p>" || editedAt.Valid || deletedAt.Valid || events != 0 || mutations != 0 {
		t.Fatalf("reply changed: %q/%q edited=%v deleted=%v events=%d mutations=%d",
			raw, rendered, editedAt, deletedAt, events, mutations)
	}
}

func newHTTPMessageService(t *testing.T) (*message.Service, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writer, err := store.NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = db.Close() })
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if err := writer.Do(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES
			(1, 'author', 'hash', 1, ?), (2, 'member', 'hash', 0, ?), (3, 'outsider', 'hash', 0, ?);
			INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at)
			VALUES (1, 'private', 'messages', 1, 'fixture', ?);
			INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (1, 1, ?), (1, 2, ?);`, now.Unix(), now.Unix(), now.Unix(), now.Unix(), now.Unix(), now.Unix()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("seed message HTTP fixtures: %v", err)
	}
	service, err := message.NewService(store.NewMessageStore(db, writer), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, db
}

func newHTTPVersionTwoReplyService(t *testing.T) (*message.Service, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "version-two-replies.db")
	legacy, err := store.Open(path, 1)
	if err != nil {
		t.Fatalf("create legacy database: %v", err)
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if _, err := legacy.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES (1, 'author', 'hash', 1, ?);
		INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at)
			VALUES (1, 'private', 'legacy', 1, 'conversation', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (1, 1, ?);
		INSERT INTO messages(id, conversation_id, author_id, reply_to_id, body, rendered_body, idempotency_key, created_at) VALUES
			(1, 1, 1, NULL, 'root', '<p>root</p>', 'root', ?),
			(2, 1, 1, 1, 'reply', '<p>reply</p>', 'reply', ?);
		DROP TABLE message_mutations;
		PRAGMA user_version = 2;`, now.Unix(), now.Unix(), now.Unix(), now.Unix(), now.Unix()); err != nil {
		t.Fatalf("seed version-two replies: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close version-two database: %v", err)
	}
	db, err := store.Open(path, 2)
	if err != nil {
		t.Fatalf("upgrade version-two database: %v", err)
	}
	writer, err := store.NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = db.Close() })
	service, err := message.NewService(store.NewMessageStore(db, writer), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, db
}
