package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestMessageStoreSendsOneOrderedEventAndReplaysOriginalResult(t *testing.T) {
	store, db := newTestMessageStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "author", "outsider")
	record := message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "hello", RenderedBody: "<p>hello</p>\n",
		IdempotencyKey: "send-1", CreatedAt: now,
	}

	first, err := store.Send(context.Background(), record)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if first.Message.ID != 1 || first.Event.Seq != 1 || first.Event.Type != "message.sent" ||
		first.Event.ConversationID != 1 || first.Event.EntityID != 1 {
		t.Fatalf("send result = %#v", first)
	}
	if !json.Valid(first.Event.Payload) {
		t.Fatalf("event payload is invalid JSON: %q", first.Event.Payload)
	}
	record.CreatedAt = now.Add(time.Hour)
	replayed, err := store.Send(context.Background(), record)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("replayed Send = (%#v, %v), want %#v", replayed, err, first)
	}
	record.Body, record.RenderedBody = "different", "<p>different</p>\n"
	if _, err := store.Send(context.Background(), record); !errors.Is(err, message.ErrConflict) {
		t.Fatalf("reused-key Send error = %v, want ErrConflict", err)
	}

	var messages, events, mutations int
	if err := db.QueryRow(`SELECT count(*),
		(SELECT count(*) FROM events), (SELECT count(*) FROM message_mutations) FROM messages`).
		Scan(&messages, &events, &mutations); err != nil {
		t.Fatalf("count send rows: %v", err)
	}
	if messages != 1 || events != 1 || mutations != 1 {
		t.Fatalf("message/event/mutation counts = %d/%d/%d, want 1/1/1", messages, events, mutations)
	}
}

func TestMessageStoreHistoryIsMembershipFilteredAndKeysetPaginated(t *testing.T) {
	store, _ := newTestMessageStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1, 2}, "author", "member", "outsider")
	for index, body := range []string{"first", "second", "third"} {
		if _, err := store.Send(context.Background(), message.SendRecord{
			ConversationID: 1, AuthorID: 1, Body: body, RenderedBody: "<p>" + body + "</p>\n",
			IdempotencyKey: "send-" + body, CreatedAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("Send(%s): %v", body, err)
		}
	}

	first, err := store.History(context.Background(), message.History{
		ConversationID: 1, UserID: 2, Limit: 2,
	})
	if err != nil || len(first.Messages) != 2 || first.Messages[0].Body != "third" ||
		first.Messages[1].Body != "second" || first.NextBeforeID != 2 {
		t.Fatalf("first history page = (%#v, %v)", first, err)
	}
	second, err := store.History(context.Background(), message.History{
		ConversationID: 1, UserID: 2, BeforeID: first.NextBeforeID, Limit: 2,
	})
	if err != nil || len(second.Messages) != 1 || second.Messages[0].Body != "first" || second.NextBeforeID != 0 {
		t.Fatalf("second history page = (%#v, %v)", second, err)
	}
	for _, query := range []message.History{
		{ConversationID: 1, UserID: 3, Limit: 50},
		{ConversationID: 99, UserID: 2, Limit: 50},
	} {
		if _, err := store.History(context.Background(), query); !errors.Is(err, message.ErrNotFound) {
			t.Errorf("History(%#v) error = %v, want ErrNotFound", query, err)
		}
	}
}

func TestMessageStoreEditsAndTombstonesOnlyForCurrentAuthorMember(t *testing.T) {
	store, db := newTestMessageStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1, 2}, "author", "other-member")
	sent, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "first", RenderedBody: "<p>first</p>\n",
		IdempotencyKey: "send", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	wrongEdit := message.EditRecord{
		MessageID: sent.Message.ID, AuthorID: 2, Body: "stolen", RenderedBody: "<p>stolen</p>\n",
		IdempotencyKey: "wrong-edit", EditedAt: now.Add(time.Minute),
	}
	if _, err := store.Edit(context.Background(), wrongEdit); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("non-author Edit error = %v, want ErrNotFound", err)
	}
	edit := message.EditRecord{
		MessageID: sent.Message.ID, AuthorID: 1, Body: "changed", RenderedBody: "<p>changed</p>\n",
		IdempotencyKey: "edit", EditedAt: now.Add(2 * time.Minute),
	}
	edited, err := store.Edit(context.Background(), edit)
	if err != nil || edited.Message.ID != sent.Message.ID || edited.Message.Body != "changed" ||
		edited.Message.EditedAt == nil || edited.Event.Seq != sent.Event.Seq+1 {
		t.Fatalf("Edit = (%#v, %v)", edited, err)
	}
	conflict := edit
	conflict.Body = "different"
	if _, err := store.Edit(context.Background(), conflict); !errors.Is(err, message.ErrConflict) {
		t.Fatalf("reused-key Edit error = %v, want ErrConflict", err)
	}
	if _, err := store.Delete(context.Background(), message.DeleteRecord{
		MessageID: sent.Message.ID, AuthorID: 2, IdempotencyKey: "wrong-delete", DeletedAt: now.Add(3 * time.Minute),
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("non-author Delete error = %v, want ErrNotFound", err)
	}
	deletion := message.DeleteRecord{
		MessageID: sent.Message.ID, AuthorID: 1, IdempotencyKey: "delete", DeletedAt: now.Add(4 * time.Minute),
	}
	deleted, err := store.Delete(context.Background(), deletion)
	if err != nil || deleted.Message.Body != "" || deleted.Message.RenderedBody != "" ||
		deleted.Message.DeletedAt == nil || deleted.Event.Seq != edited.Event.Seq+1 {
		t.Fatalf("Delete = (%#v, %v)", deleted, err)
	}
	replayedDelete, err := store.Delete(context.Background(), deletion)
	if err != nil || !reflect.DeepEqual(replayedDelete, deleted) {
		t.Fatalf("replayed Delete = (%#v, %v), want %#v", replayedDelete, err, deleted)
	}
	replayedEdit, err := store.Edit(context.Background(), edit)
	if err != nil || !reflect.DeepEqual(replayedEdit, edited) {
		t.Fatalf("post-delete replayed Edit = (%#v, %v), want %#v", replayedEdit, err, edited)
	}
	edit.IdempotencyKey = "edit-after-delete"
	if _, err := store.Edit(context.Background(), edit); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("Edit tombstone error = %v, want ErrNotFound", err)
	}

	page, err := store.History(context.Background(), message.History{ConversationID: 1, UserID: 1, Limit: 50})
	if err != nil || len(page.Messages) != 1 || page.Messages[0].ID != sent.Message.ID ||
		page.Messages[0].Body != "" || page.Messages[0].RenderedBody != "" {
		t.Fatalf("tombstone history = (%#v, %v)", page, err)
	}
	var raw, rendered string
	var events int
	if err := db.QueryRow(`SELECT body, rendered_body,
		(SELECT count(*) FROM events) FROM messages WHERE id = ?`, sent.Message.ID).
		Scan(&raw, &rendered, &events); err != nil {
		t.Fatalf("read tombstone: %v", err)
	}
	if raw != "" || rendered != "" || events != 3 {
		t.Fatalf("stored tombstone raw/rendered/events = %q/%q/%d", raw, rendered, events)
	}
}

func TestMessageStoreScopesIdempotencyToActorAndRequiresMembership(t *testing.T) {
	store, _ := newTestMessageStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1, 2}, "first", "second", "outsider")
	for userID := int64(1); userID <= 2; userID++ {
		result, err := store.Send(context.Background(), message.SendRecord{
			ConversationID: 1, AuthorID: userID, Body: "actor", RenderedBody: "<p>actor</p>\n",
			IdempotencyKey: "shared-key", CreatedAt: now,
		})
		if err != nil || result.Message.ID != userID || result.Event.Seq != userID {
			t.Fatalf("actor %d Send = (%#v, %v)", userID, result, err)
		}
	}
	if _, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 3, Body: "outsider", RenderedBody: "<p>outsider</p>\n",
		IdempotencyKey: "outsider", CreatedAt: now,
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("outsider Send error = %v, want ErrNotFound", err)
	}
}

func TestMessageStoreRollsBackDomainAndEventWhenMutationSnapshotFails(t *testing.T) {
	store, db := newTestMessageStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "author")
	if _, err := db.Exec(`CREATE TRIGGER fail_message_mutation
		BEFORE INSERT ON message_mutations
		BEGIN SELECT RAISE(FAIL, 'forced mutation failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "rollback", RenderedBody: "<p>rollback</p>\n",
		IdempotencyKey: "rollback", CreatedAt: now,
	}); err == nil {
		t.Fatal("Send error = nil, want direct SQL failure")
	}
	var messages, events, mutations int
	if err := db.QueryRow(`SELECT count(*),
		(SELECT count(*) FROM events), (SELECT count(*) FROM message_mutations) FROM messages`).
		Scan(&messages, &events, &mutations); err != nil {
		t.Fatalf("count rolled-back rows: %v", err)
	}
	if messages != 0 || events != 0 || mutations != 0 {
		t.Fatalf("rolled-back message/event/mutation counts = %d/%d/%d", messages, events, mutations)
	}
}

func newTestMessageStore(t *testing.T) (*MessageStore, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	writer, err := NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})
	return NewMessageStore(db, writer), db
}

func seedMessageFixtures(t *testing.T, writer *Writer, now time.Time, memberIDs []int64, usernames ...string) {
	t.Helper()
	seedConversationUsers(t, writer, now, usernames...)
	if err := writer.Do(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO conversations(
			id, kind, name, created_by, idempotency_key, created_at)
			VALUES (1, 'private', 'messages', 1, 'fixture', ?)`, now.Unix()); err != nil {
			return err
		}
		for _, userID := range memberIDs {
			if _, err := tx.Exec(`INSERT INTO conversation_members(conversation_id, user_id, joined_at)
				VALUES (1, ?, ?)`, userID, now.Unix()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed message conversation: %v", err)
	}
}
