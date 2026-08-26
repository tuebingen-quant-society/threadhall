package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func TestConversationStoreCreatesNamedChannelsWithCreatorMembership(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "creator")

	for index, kind := range []conversation.Kind{conversation.KindChannel, conversation.KindPrivate} {
		created, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
			CreatorID: 1, Kind: kind, Name: string(kind), IdempotencyKey: "create-" + string(kind),
			CreatedAt: now,
		})
		if err != nil {
			t.Fatalf("CreateChannel(%s): %v", kind, err)
		}
		detail, err := store.Detail(context.Background(), 1, created.ID)
		if err != nil || detail.Kind != kind {
			t.Fatalf("Detail(%s) = (%#v, %v)", kind, detail, err)
		}
		members, err := store.ListMembers(context.Background(), 1, created.ID, 0, 1)
		if err != nil || len(members.Members) != 1 || members.Members[0].UserID != 1 {
			t.Fatalf("Members(%s) = (%#v, %v)", kind, members, err)
		}
		if created.ID != int64(index+1) {
			t.Fatalf("created ID = %d, want %d", created.ID, index+1)
		}
	}

	var events int
	if err := db.QueryRow("SELECT count(*) FROM events WHERE kind = 'conversation.created'").Scan(&events); err != nil || events != 2 {
		t.Fatalf("created events = (%d, %v), want 2", events, err)
	}
}

func TestConversationStoreRenamesOwnedChannelIdempotently(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 19, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "owner", "member")
	created, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{CreatorID: 1, Kind: conversation.KindPrivate, Name: "before", IdempotencyKey: "create-before", CreatedAt: now})
	if err != nil { t.Fatal(err) }
	record := conversation.RenameRecord{ActorID: 1, ConversationID: created.ID, Name: "after", IdempotencyKey: "rename-after", RenamedAt: now}
	renamed, err := store.RenameConversation(context.Background(), record)
	if err != nil || renamed.Name != "after" { t.Fatalf("RenameConversation = (%#v, %v)", renamed, err) }
	replayed, err := store.RenameConversation(context.Background(), record)
	if err != nil || replayed.Name != "after" { t.Fatalf("replayed rename = (%#v, %v)", replayed, err) }
	if _, err := store.RenameConversation(context.Background(), conversation.RenameRecord{ActorID: 2, ConversationID: created.ID, Name: "nope", IdempotencyKey: "member-rename", RenamedAt: now}); !errors.Is(err, conversation.ErrForbidden) { t.Fatalf("member rename error = %v", err) }
	var events int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE kind = 'conversation.renamed'`).Scan(&events); err != nil || events != 1 { t.Fatalf("rename events = %d, %v", events, err) }
}

func TestConversationStoreListsAndOpensOnlyMembershipsWithKeysetBounds(t *testing.T) {
	store, _ := newTestConversationStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "creator", "outsider")
	for index, name := range []string{"first", "second", "third"} {
		if _, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
			CreatorID: 1, Kind: conversation.KindPrivate, Name: name,
			IdempotencyKey: "channel-" + name, CreatedAt: now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("CreateChannel(%s): %v", name, err)
		}
	}

	first, err := store.List(context.Background(), 1, 0, 2)
	if err != nil || len(first.Conversations) != 2 || first.Conversations[0].Name != "third" || first.NextBeforeID != 2 {
		t.Fatalf("first page = (%#v, %v)", first, err)
	}
	second, err := store.List(context.Background(), 1, first.NextBeforeID, 2)
	if err != nil || len(second.Conversations) != 1 || second.Conversations[0].Name != "first" || second.NextBeforeID != 0 {
		t.Fatalf("second page = (%#v, %v)", second, err)
	}
	outsider, err := store.List(context.Background(), 2, 0, 100)
	if err != nil || len(outsider.Conversations) != 0 {
		t.Fatalf("outsider list = (%#v, %v)", outsider, err)
	}
	if _, err := store.Detail(context.Background(), 2, 1); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("outsider Detail error = %v, want ErrNotFound", err)
	}
	if allowed, err := store.CanRead(context.Background(), 2, 1); err != nil || allowed {
		t.Fatalf("outsider CanRead = (%t, %v)", allowed, err)
	}
}

func TestConversationStoreMakesChannelCreationIdempotentAndNamesCaseInsensitive(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "creator")
	record := conversation.ChannelRecord{
		CreatorID: 1, Kind: conversation.KindChannel, Name: "General",
		IdempotencyKey: "general-key", CreatedAt: now,
	}
	first, err := store.CreateChannel(context.Background(), record)
	if err != nil {
		t.Fatalf("first CreateChannel: %v", err)
	}
	replayed, err := store.CreateChannel(context.Background(), record)
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("replayed CreateChannel = (%#v, %v)", replayed, err)
	}
	record.Name = "Different"
	if _, err := store.CreateChannel(context.Background(), record); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("reused-key CreateChannel error = %v, want ErrConflict", err)
	}
	record.Name, record.IdempotencyKey = "general", "case-collision"
	if _, err := store.CreateChannel(context.Background(), record); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("case-collision CreateChannel error = %v, want ErrConflict", err)
	}
	var conversations, events int
	if err := db.QueryRow("SELECT count(*) FROM conversations").Scan(&conversations); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if err := db.QueryRow("SELECT count(*) FROM events").Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if conversations != 1 || events != 1 {
		t.Fatalf("idempotent row/event counts = %d/%d, want 1/1", conversations, events)
	}
}

func TestConversationStoreCanonicalizesOneDirectMessageWithExactlyTwoExistingMembers(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "admin", "member", "third")
	first, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 1, OtherUserID: 2, UserLowID: 1, UserHighID: 2,
		IdempotencyKey: "dm-from-one", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("first CreateDM: %v", err)
	}
	equivalent, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 2, OtherUserID: 1, UserLowID: 1, UserHighID: 2,
		IdempotencyKey: "dm-from-two", CreatedAt: now.Add(time.Second),
	})
	if err != nil || equivalent.ID != first.ID {
		t.Fatalf("equivalent CreateDM = (%#v, %v)", equivalent, err)
	}
	replayed, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 1, OtherUserID: 2, UserLowID: 1, UserHighID: 2,
		IdempotencyKey: "dm-from-one", CreatedAt: now.Add(2 * time.Second),
	})
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("replayed CreateDM = (%#v, %v)", replayed, err)
	}
	members, err := store.ListMembers(context.Background(), 1, first.ID, 0, 100)
	if err != nil || len(members.Members) != 2 || members.Members[0].UserID != 2 || members.Members[1].UserID != 1 {
		t.Fatalf("DM members = (%#v, %v)", members, err)
	}
	adminPage, err := store.List(context.Background(), 1, 0, 10)
	if err != nil || adminPage.Conversations[0].PeerUsername != "member" {
		t.Fatalf("admin DM label = (%#v, %v)", adminPage, err)
	}
	memberDetail, err := store.Detail(context.Background(), 2, first.ID)
	if err != nil || memberDetail.PeerUsername != "admin" {
		t.Fatalf("member DM label = (%#v, %v)", memberDetail, err)
	}
	if _, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 1, OtherUserID: 3, UserLowID: 1, UserHighID: 3,
		IdempotencyKey: "dm-from-one", CreatedAt: now,
	}); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("reused-key CreateDM error = %v, want ErrConflict", err)
	}
	if _, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 1, OtherUserID: 99, UserLowID: 1, UserHighID: 99,
		IdempotencyKey: "missing-user", CreatedAt: now,
	}); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("missing-user CreateDM error = %v, want ErrNotFound", err)
	}
	var conversations, membersCount, events int
	if err := db.QueryRow("SELECT count(*), (SELECT count(*) FROM conversation_members), (SELECT count(*) FROM events) FROM conversations").Scan(&conversations, &membersCount, &events); err != nil {
		t.Fatalf("count DM rows: %v", err)
	}
	if conversations != 1 || membersCount != 2 || events != 1 {
		t.Fatalf("DM row/member/event counts = %d/%d/%d, want 1/2/1", conversations, membersCount, events)
	}
}

func TestConversationStoreLetsOnlyWorkspaceAdminsManageNamedChannelMembers(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "admin", "creator", "target", "other")
	channel, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 2, Kind: conversation.KindPrivate, Name: "staff",
		IdempotencyKey: "staff", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	ordinary := conversation.MemberRecord{
		ActorID: 2, ConversationID: channel.ID, UserID: 3,
		IdempotencyKey: "ordinary-add", ChangedAt: now,
	}
	if err := store.AddMember(context.Background(), ordinary); !errors.Is(err, conversation.ErrForbidden) {
		t.Fatalf("ordinary AddMember error = %v, want ErrForbidden", err)
	}
	admin := ordinary
	admin.ActorID, admin.IdempotencyKey = 1, "admin-add"
	if err := store.AddMember(context.Background(), admin); err != nil {
		t.Fatalf("admin AddMember: %v", err)
	}
	if err := store.AddMember(context.Background(), admin); err != nil {
		t.Fatalf("replayed AddMember: %v", err)
	}
	admin.UserID = 4
	if err := store.AddMember(context.Background(), admin); !errors.Is(err, conversation.ErrConflict) {
		t.Fatalf("reused-key AddMember error = %v, want ErrConflict", err)
	}
	if _, err := store.Detail(context.Background(), 3, channel.ID); err != nil {
		t.Fatalf("added member Detail: %v", err)
	}
	admin.UserID, admin.IdempotencyKey = 3, "admin-remove"
	if err := store.RemoveMember(context.Background(), admin); err != nil {
		t.Fatalf("admin RemoveMember: %v", err)
	}
	if err := store.RemoveMember(context.Background(), admin); err != nil {
		t.Fatalf("replayed RemoveMember: %v", err)
	}
	if _, err := store.Detail(context.Background(), 3, channel.ID); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("removed member Detail error = %v, want ErrNotFound", err)
	}

	dm, err := store.CreateDM(context.Background(), conversation.DMRecord{
		RequesterID: 1, OtherUserID: 2, UserLowID: 1, UserHighID: 2,
		IdempotencyKey: "admin-creator-dm", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateDM: %v", err)
	}
	admin.ConversationID, admin.IdempotencyKey = dm.ID, "dm-add"
	if err := store.AddMember(context.Background(), admin); !errors.Is(err, conversation.ErrNotFound) {
		t.Fatalf("DM AddMember error = %v, want ErrNotFound", err)
	}
	var membershipEvents int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE kind IN ('conversation.member_added', 'conversation.member_removed')`).Scan(&membershipEvents); err != nil || membershipEvents != 2 {
		t.Fatalf("membership events = (%d, %v), want 2", membershipEvents, err)
	}
}

func TestConversationStoreKeepsAgentMembershipUnderGrantAdministration(t *testing.T) {
	store, db := newTestConversationStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedConversationUsers(t, store.writer, now, "admin", "creator", "codex")
	channel, err := store.CreateChannel(context.Background(), conversation.ChannelRecord{
		CreatorID: 2, Kind: conversation.KindPrivate, Name: "scoped", IdempotencyKey: "scoped", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if _, err := db.Exec(`UPDATE users SET principal_kind = 'agent' WHERE id = 3;
		INSERT INTO agents(user_id, token_hash, created_by, created_at) VALUES (3, zeroblob(32), 1, ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (?, 3, ?)`,
		now.Unix(), channel.ID, now.Unix()); err != nil {
		t.Fatalf("seed agent membership: %v", err)
	}
	members, err := store.ListMembers(context.Background(), 2, channel.ID, 0, 100)
	if err != nil || len(members.Members) != 2 || members.Members[0].PrincipalKind != "agent" {
		t.Fatalf("agent member projection = (%#v, %v), want agent kind", members, err)
	}
	for _, change := range []struct {
		name string
		call func(context.Context, conversation.MemberRecord) error
	}{
		{"add", store.AddMember},
		{"remove", store.RemoveMember},
	} {
		err := change.call(context.Background(), conversation.MemberRecord{
			ActorID: 1, ConversationID: channel.ID, UserID: 3,
			IdempotencyKey: "agent-" + change.name, ChangedAt: now,
		})
		if !errors.Is(err, conversation.ErrNotFound) {
			t.Fatalf("%s agent membership error = %v, want ErrNotFound", change.name, err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM conversation_members WHERE conversation_id = ? AND user_id = 3`, channel.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("agent membership count = %d, %v; want preserved", count, err)
	}
}

func newTestConversationStore(t *testing.T) (*ConversationStore, *sql.DB) {
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
	return NewConversationStore(db, writer), db
}

func seedConversationUsers(t *testing.T, writer *Writer, now time.Time, usernames ...string) {
	t.Helper()
	if err := writer.Do(context.Background(), func(tx *sql.Tx) error {
		for index, username := range usernames {
			_, err := tx.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at)
				VALUES (?, ?, 'hash', ?, ?)`, index+1, username, index == 0, now.Unix())
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed users: %v", err)
	}
}
