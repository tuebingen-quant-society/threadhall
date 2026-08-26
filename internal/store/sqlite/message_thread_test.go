package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestMessageStorePersistsOneLevelAuthorizedThreads(t *testing.T) {
	store, _ := newTestMessageStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1, 2}, "author", "member")
	root := sendThreadFixture(t, store, message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "root", RenderedBody: "<p>root</p>", IdempotencyKey: "root", CreatedAt: now,
	})
	reply := sendThreadFixture(t, store, message.SendRecord{
		ConversationID: 1, AuthorID: 2, ThreadRootID: &root.Message.ID, Body: "reply", RenderedBody: "<p>reply</p>", IdempotencyKey: "reply", CreatedAt: now.Add(time.Second),
	})

	rootPage, err := store.History(context.Background(), message.History{ConversationID: 1, UserID: 1, Limit: 20})
	if err != nil || len(rootPage.Messages) != 1 || rootPage.Messages[0].ID != root.Message.ID {
		t.Fatalf("root history = (%#v, %v)", rootPage, err)
	}
	thread, err := store.Thread(context.Background(), message.Thread{ConversationID: 1, RootMessageID: root.Message.ID, UserID: 1, Limit: 20})
	if err != nil || thread.Root.ID != root.Message.ID || len(thread.Replies) != 1 || thread.Replies[0].ID != reply.Message.ID {
		t.Fatalf("thread = (%#v, %v)", thread, err)
	}
	if thread.Replies[0].ThreadRootID == nil || *thread.Replies[0].ThreadRootID != root.Message.ID {
		t.Fatalf("reply root = %#v", thread.Replies[0].ThreadRootID)
	}
	summaries, err := store.Threads(context.Background(), message.ListThreads{ConversationID: 1, UserID: 1, Limit: 20})
	if err != nil || len(summaries.Threads) != 1 || summaries.Threads[0].Root.ID != root.Message.ID || summaries.Threads[0].ReplyCount != 1 {
		t.Fatalf("Threads = (%#v, %v)", summaries, err)
	}

	if _, err := store.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, ThreadRootID: &reply.Message.ID, Body: "nested", RenderedBody: "<p>nested</p>", IdempotencyKey: "nested", CreatedAt: now,
	}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("nested reply error = %v, want ErrNotFound", err)
	}
	if _, err := store.Thread(context.Background(), message.Thread{ConversationID: 1, RootMessageID: root.Message.ID, UserID: 99, Limit: 20}); !errors.Is(err, message.ErrNotFound) {
		t.Fatalf("unauthorized thread error = %v, want ErrNotFound", err)
	}
}

func TestMessageStoreDeletesWholeThreadForRootAuthorOwnerOrAdmin(t *testing.T) {
	store, db := newTestMessageStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1, 2, 3}, "owner", "root-author", "member")
	root := sendThreadFixture(t, store, message.SendRecord{ConversationID: 1, AuthorID: 2, Body: "root", RenderedBody: "<p>root</p>", IdempotencyKey: "delete-root", CreatedAt: now})
	sendThreadFixture(t, store, message.SendRecord{ConversationID: 1, AuthorID: 3, ThreadRootID: &root.Message.ID, Body: "reply", RenderedBody: "<p>reply</p>", IdempotencyKey: "delete-reply", CreatedAt: now})

	if err := store.DeleteThread(context.Background(), 3, 1, root.Message.ID); !errors.Is(err, message.ErrForbidden) {
		t.Fatalf("member DeleteThread error = %v, want ErrForbidden", err)
	}
	if err := store.DeleteThread(context.Background(), 2, 1, root.Message.ID); err != nil {
		t.Fatalf("author DeleteThread: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM messages WHERE id = ? OR thread_root_id = ?`, root.Message.ID, root.Message.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("thread message count = %d, %v; want 0", count, err)
	}
}

func sendThreadFixture(t *testing.T, store *MessageStore, record message.SendRecord) message.Result {
	t.Helper()
	result, err := store.Send(context.Background(), record)
	if err != nil {
		t.Fatalf("Send(%q): %v", record.Body, err)
	}
	return result
}
