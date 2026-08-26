package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestConversationStoreForksThreadIntoIndependentChannel(t *testing.T) {
	conversationStore, _ := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, conversationStore.writer, now, "creator", "member", "outsider")
	source, err := conversationStore.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "source", IdempotencyKey: "source", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if err := conversationStore.AddMember(context.Background(), conversation.MemberRecord{
		ActorID: 1, ConversationID: source.ID, UserID: 2, IdempotencyKey: "add-member", ChangedAt: now,
	}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	messages := NewMessageStore(conversationStore.db, conversationStore.writer)
	root := sendThreadFixture(t, messages, message.SendRecord{ConversationID: source.ID, AuthorID: 1, Body: "root", RenderedBody: "<p>root</p>", IdempotencyKey: "root", CreatedAt: now})
	reply := sendThreadFixture(t, messages, message.SendRecord{ConversationID: source.ID, AuthorID: 2, ThreadRootID: &root.Message.ID, Body: "reply", RenderedBody: "<p>reply</p>", IdempotencyKey: "reply", CreatedAt: now})

	fork, err := conversationStore.Fork(context.Background(), conversation.ForkRecord{
		ActorID: 2, SourceConversationID: source.ID, SourceMessageID: reply.Message.ID,
		Kind: conversation.KindPrivate, Name: "forked-work", IdempotencyKey: "fork", CreatedAt: now,
	})
	if err != nil || fork.SourceRootMessageID != root.Message.ID || fork.Conversation.Name != "forked-work" {
		t.Fatalf("Fork = (%#v, %v)", fork, err)
	}
	if _, err := conversationStore.Detail(context.Background(), 1, fork.Conversation.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("source creator inherited fork membership: %v", err)
	}
	if _, err := conversationStore.Detail(context.Background(), 2, fork.Conversation.ID); err != nil {
		t.Fatalf("fork creator Detail: %v", err)
	}
	replayed, err := conversationStore.Fork(context.Background(), conversation.ForkRecord{
		ActorID: 2, SourceConversationID: source.ID, SourceMessageID: reply.Message.ID,
		Kind: conversation.KindPrivate, Name: "forked-work", IdempotencyKey: "fork", CreatedAt: now.Add(time.Second),
	})
	if err != nil || replayed.Conversation.ID != fork.Conversation.ID {
		t.Fatalf("replayed Fork = (%#v, %v)", replayed, err)
	}
	if _, err := conversationStore.Fork(context.Background(), conversation.ForkRecord{
		ActorID: 3, SourceConversationID: source.ID, SourceMessageID: root.Message.ID,
		Kind: conversation.KindPrivate, Name: "forbidden", IdempotencyKey: "forbidden", CreatedAt: now,
	}); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("unauthorized Fork error = %v, want ErrNotFound", err)
	}
}
