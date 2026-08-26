package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func TestPublicChannelAutomaticallyGrantsActiveAgents(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 18, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "admin")
	seedActiveAgent(t, db, now)

	public, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindChannel, Name: "public",
		IdempotencyKey: "public", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel(public): %v", err)
	}
	private, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "private",
		IdempotencyKey: "private", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel(private): %v", err)
	}

	assertAgentAccess(t, db, public.ID, 1)
	assertAgentAccess(t, db, private.ID, 0)
}

func TestNewAgentAutomaticallyJoinsExistingPublicChannels(t *testing.T) {
	store, _, db := newTestAgentStore(t)
	now := time.Date(2026, 8, 26, 18, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "admin")
	conversations := NewConversationStore(db, store.writer)

	public, err := conversations.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindChannel, Name: "public",
		IdempotencyKey: "public", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel(public): %v", err)
	}
	private, err := conversations.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "private",
		IdempotencyKey: "private", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel(private): %v", err)
	}

	token := sha256.Sum256([]byte("worker token"))
	if _, err := store.Create(context.Background(), agenttask.CreateAgent{
		Username: "codex", TokenHash: token, CreatedBy: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("Create(agent): %v", err)
	}

	assertAgentAccess(t, db, public.ID, 1)
	assertAgentAccess(t, db, private.ID, 0)
}

func seedActiveAgent(t *testing.T, db *sql.DB, now time.Time) {
	t.Helper()
	token := sha256.Sum256([]byte("worker token"))
	if _, err := db.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at, principal_kind)
		VALUES (2, 'codex', '!', 0, ?, 'agent');
		INSERT INTO agents(user_id, token_hash, created_by, created_at) VALUES (2, ?, 1, ?)`,
		now.Unix(), token[:], now.Unix()); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
}

func assertAgentAccess(t *testing.T, db *sql.DB, conversationID int64, want int) {
	t.Helper()
	var membership, grant int
	if err := db.QueryRow(`SELECT
		EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id = ? AND user_id = 2),
		EXISTS(SELECT 1 FROM agent_conversation_grants WHERE conversation_id = ? AND agent_id = 2)`,
		conversationID, conversationID).Scan(&membership, &grant); err != nil {
		t.Fatalf("read agent access: %v", err)
	}
	if membership != want || grant != want {
		t.Fatalf("agent membership/grant = %d/%d, want %d/%d", membership, grant, want, want)
	}
}
