package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestConversationStoreCreatesPrivateChannelWithExplicitHumanMembers(t *testing.T) {
	store, _ := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "creator", "member", "other")

	created, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "deal-room",
		MemberIDs: []int64{2}, IdempotencyKey: "deal-room", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	members, err := store.ListMembers(context.Background(), 1, created.ID, 0, 10)
	if err != nil || len(members.Members) != 2 || members.Members[0].UserID != 2 || members.Members[1].UserID != 1 {
		t.Fatalf("members = (%#v, %v), want creator and selected member", members, err)
	}
	if _, err := store.Detail(context.Background(), 3, created.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("unselected member Detail error = %v, want ErrNotFound", err)
	}
}

func TestConversationStoreRejectsMissingPrivateChannelMemberAtomically(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "creator")

	_, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "deal-room",
		MemberIDs: []int64{99}, IdempotencyKey: "deal-room", CreatedAt: now,
	})
	if !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("CreateChannel error = %v, want ErrNotFound", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM conversations`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("conversation count = %d, %v; want rolled back", count, err)
	}
}

func TestConversationStoreDeletesNamedChannelForOwnerOrAdmin(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "admin", "owner", "member")
	created, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 2, Kind: conversation.KindPrivate, Name: "delete-me", MemberIDs: []int64{3},
		IdempotencyKey: "delete-me", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if err := store.DeleteConversation(context.Background(), 3, created.ID); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("member delete error = %v, want ErrForbidden", err)
	}
	if err := store.DeleteConversation(context.Background(), 2, created.ID); err != nil {
		t.Fatalf("owner DeleteConversation: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM conversations WHERE id = ?`, created.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("conversation count = %d, %v; want 0", count, err)
	}
}

func TestConversationUnreadCountsIgnoreOwnAndThreadRepliesUntilTheirViewsAreRead(t *testing.T) {
	conversationStore, _ := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, conversationStore.writer, now, "author", "reader")
	channel, err := conversationStore.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "unread", MemberIDs: []int64{2},
		IdempotencyKey: "unread", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	messages := NewMessageStore(conversationStore.db, conversationStore.writer)
	root, err := messages.Send(context.Background(), message.SendRecord{ConversationID: channel.ID, AuthorID: 1, Body: "root", RenderedBody: "<p>root</p>", IdempotencyKey: "root", CreatedAt: now})
	if err != nil {
		t.Fatalf("Send root: %v", err)
	}
	if _, err := messages.Send(context.Background(), message.SendRecord{ConversationID: channel.ID, AuthorID: 1, ThreadRootID: &root.Message.ID, Body: "reply", RenderedBody: "<p>reply</p>", IdempotencyKey: "reply", CreatedAt: now.Add(time.Second)}); err != nil {
		t.Fatalf("Send reply: %v", err)
	}
	if _, err := messages.Send(context.Background(), message.SendRecord{ConversationID: channel.ID, AuthorID: 2, Body: "own", RenderedBody: "<p>own</p>", IdempotencyKey: "own", CreatedAt: now.Add(2 * time.Second)}); err != nil {
		t.Fatalf("Send own: %v", err)
	}

	page, err := conversationStore.List(context.Background(), 2, 0, 10)
	if err != nil || page.Conversations[0].UnreadCount != 1 {
		t.Fatalf("conversation unread = (%#v, %v), want 1", page, err)
	}
	threads, err := messages.Threads(context.Background(), message.ListThreads{ConversationID: channel.ID, UserID: 2, Limit: 10})
	if err != nil || threads.Threads[0].UnreadCount != 1 {
		t.Fatalf("thread unread = (%#v, %v), want 1", threads, err)
	}
	if err := conversationStore.MarkRead(context.Background(), 2, channel.ID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := messages.MarkThreadRead(context.Background(), 2, channel.ID, root.Message.ID, now.Add(3*time.Second)); err != nil {
		t.Fatalf("MarkThreadRead: %v", err)
	}
	page, _ = conversationStore.List(context.Background(), 2, 0, 10)
	threads, _ = messages.Threads(context.Background(), message.ListThreads{ConversationID: channel.ID, UserID: 2, Limit: 10})
	if page.Conversations[0].UnreadCount != 0 || threads.Threads[0].UnreadCount != 0 {
		t.Fatalf("read counts = conversation %d thread %d, want 0/0", page.Conversations[0].UnreadCount, threads.Threads[0].UnreadCount)
	}
}
