package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestMessageStorePersistsSameLaneReplyReferences(t *testing.T) {
	t.Parallel()
	store, _ := newTestMessageStore(t)
	now := time.Date(2026, 8, 26, 13, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "author")
	original, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "original", RenderedBody: "<p>original</p>", IdempotencyKey: "original", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("send original: %v", err)
	}
	reply, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, ReplyToMessageID: &original.Message.ID,
		Body: "linked reply", RenderedBody: "<p>linked reply</p>", IdempotencyKey: "reply", CreatedAt: now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("send reply: %v", err)
	}
	if reply.Message.ReplyToMessageID == nil || *reply.Message.ReplyToMessageID != original.Message.ID {
		t.Fatalf("reply reference = %#v", reply.Message.ReplyToMessageID)
	}
	var payload map[string]any
	if err := json.Unmarshal(reply.Event.Payload, &payload); err != nil || payload["reply_to_message_id"] != float64(original.Message.ID) {
		t.Fatalf("reply event payload = (%#v, %v)", payload, err)
	}
	page, err := store.History(context.Background(), message.History{ConversationID: 1, UserID: 1, Limit: 10})
	if err != nil || len(page.Messages) != 2 || page.Messages[0].ReplyToMessageID == nil || *page.Messages[0].ReplyToMessageID != original.Message.ID {
		t.Fatalf("reply history = (%#v, %v)", page, err)
	}
	missing := int64(999)
	if _, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, ReplyToMessageID: &missing,
		Body: "invalid", RenderedBody: "<p>invalid</p>", IdempotencyKey: "invalid", CreatedAt: now,
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("missing reply target error = %v, want ErrNotFound", err)
	}
}

func TestMessageStoreRestrictsReplyReferencesToCurrentThreadLane(t *testing.T) {
	t.Parallel()
	store, _ := newTestMessageStore(t)
	now := time.Date(2026, 8, 26, 13, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "author")
	root := sendThreadFixture(t, store, message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "root", RenderedBody: "<p>root</p>", IdempotencyKey: "root", CreatedAt: now,
	})
	other := sendThreadFixture(t, store, message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "other", RenderedBody: "<p>other</p>", IdempotencyKey: "other", CreatedAt: now,
	})
	if _, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, ThreadRootID: &root.Message.ID, ReplyToMessageID: &other.Message.ID,
		Body: "cross lane", RenderedBody: "<p>cross lane</p>", IdempotencyKey: "cross", CreatedAt: now,
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("cross-lane reply error = %v, want ErrNotFound", err)
	}
}
