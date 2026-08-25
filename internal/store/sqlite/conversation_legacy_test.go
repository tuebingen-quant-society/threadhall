package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func TestConversationStoreAdoptsOnlyExactLegacyChannelIdempotency(t *testing.T) {
	db := openUpgradedLegacyIdempotencyDB(t)
	writer, err := NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	store := NewConversationStore(db, writer)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	replayed, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindChannel, Name: "Legacy",
		IdempotencyKey: "legacy-exact", CreatedAt: now.Add(time.Hour),
	})
	if err != nil || replayed.ID != 7 || !replayed.CreatedAt.Equal(now) {
		t.Fatalf("exact legacy retry = (%#v, %v)", replayed, err)
	}
	var adopted int
	if err := db.QueryRow(`SELECT count(*) FROM conversation_mutations
		WHERE actor_id = 1 AND idempotency_key = 'legacy-exact'
		AND operation = 'create_channel' AND conversation_id = 7`).Scan(&adopted); err != nil || adopted != 1 {
		t.Fatalf("adopted ledger rows = (%d, %v), want 1", adopted, err)
	}

	if _, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindChannel, Name: "Different",
		IdempotencyKey: "legacy-mismatch", CreatedAt: now,
	}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("mismatched legacy retry error = %v, want ErrConflict", err)
	}
	if _, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 1, OtherUserID: 2, UserLowID: 1, UserHighID: 2,
		IdempotencyKey: "legacy-dm-cross", CreatedAt: now,
	}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("legacy key reused for DM error = %v, want ErrConflict", err)
	}
	if err := store.AddMember(context.Background(), conversation.MemberRecord{
		ActorID: 1, ConversationID: 8, UserID: 3,
		IdempotencyKey: "legacy-member-cross", ChangedAt: now,
	}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("legacy key reused for membership error = %v, want ErrConflict", err)
	}
	var targetMembership int
	if err := db.QueryRow(`SELECT count(*) FROM conversation_members
		WHERE conversation_id = 8 AND user_id = 3`).Scan(&targetMembership); err != nil || targetMembership != 0 {
		t.Fatalf("cross-operation membership rows = (%d, %v), want 0", targetMembership, err)
	}
}

func TestConversationStoreAdoptsOnlyExactLegacyDMIdempotency(t *testing.T) {
	db := openUpgradedLegacyIdempotencyDB(t)
	writer, err := NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	store := NewConversationStore(db, writer)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

	if _, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 2, OtherUserID: 3, UserLowID: 2, UserHighID: 3,
		IdempotencyKey: "dm-origin", CreatedAt: now.Add(time.Hour),
	}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("mismatched legacy DM retry error = %v, want ErrConflict", err)
	}
	if _, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 2, Kind: conversation.KindChannel, Name: "Wrong operation",
		IdempotencyKey: "dm-origin", CreatedAt: now.Add(time.Hour),
	}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("legacy DM key reused for channel error = %v, want ErrConflict", err)
	}

	replayed, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 2, OtherUserID: 1, UserLowID: 1, UserHighID: 2,
		IdempotencyKey: "dm-origin", CreatedAt: now.Add(time.Hour),
	})
	if err != nil || replayed.ID != 10 || replayed.Kind != conversation.KindDM || !replayed.CreatedAt.Equal(now) {
		t.Fatalf("exact legacy DM retry = (%#v, %v)", replayed, err)
	}
	var mutations, memberships, events int
	if err := db.QueryRow(`SELECT count(*) FROM conversation_mutations
		WHERE actor_id = 2 AND idempotency_key = 'dm-origin'
		AND operation = 'create_dm' AND conversation_id = 10`).Scan(&mutations); err != nil {
		t.Fatalf("count adopted DM mutations: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM conversation_members WHERE conversation_id = 10`).Scan(&memberships); err != nil {
		t.Fatalf("count legacy DM memberships: %v", err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE conversation_id = 10`).Scan(&events); err != nil {
		t.Fatalf("count legacy DM events: %v", err)
	}
	if mutations != 1 || memberships != 2 || events != 0 {
		t.Fatalf("legacy DM side effects = mutations %d memberships %d events %d", mutations, memberships, events)
	}
}

func openUpgradedLegacyIdempotencyDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy-idempotency.db")
	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	script, err := migrationFiles.ReadFile(migrations[0])
	if err != nil {
		t.Fatalf("read core migration: %v", err)
	}
	if err := applyMigration(context.Background(), legacy, 1, string(script)); err != nil {
		t.Fatalf("apply core migration: %v", err)
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC).Unix()
	if _, err := legacy.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES
			(1, 'admin', 'hash', 1, ?), (2, 'member', 'hash', 0, ?), (3, 'target', 'hash', 0, ?);
		INSERT INTO conversations(id, kind, name, created_by, dm_user_low, dm_user_high, idempotency_key, created_at) VALUES
			(7, 'channel', 'Legacy', 1, NULL, NULL, 'legacy-exact', ?),
			(8, 'private', 'Staff', 1, NULL, NULL, 'legacy-mismatch', ?),
			(9, 'channel', 'Ops', 1, NULL, NULL, 'legacy-dm-cross', ?),
			(10, 'dm', NULL, 2, 1, 2, 'dm-origin', ?),
			(11, 'channel', 'Members', 1, NULL, NULL, 'legacy-member-cross', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES
			(7, 1, ?), (8, 1, ?), (9, 1, ?), (10, 1, ?), (10, 2, ?), (11, 1, ?);`,
		now, now, now, now, now, now, now, now, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed legacy idempotency rows: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	db, err := Open(path, 2)
	if err != nil {
		t.Fatalf("upgrade legacy database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
