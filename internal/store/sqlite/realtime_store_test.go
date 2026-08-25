package sqlite

import (
	"context"
	"database/sql"
	"testing"
)

func TestReplayStoreUsesGlobalBoundsAndCurrentConversationMembership(t *testing.T) {
	db := openTestDB(t)
	seedReplayFixtures(t, db)
	store := NewReplayStore(db)
	memberships, err := store.Memberships(context.Background(), 1)
	if err != nil || len(memberships) != 1 || memberships[0] != 1 {
		t.Fatalf("Memberships = (%v, %v), want [1]", memberships, err)
	}

	minimum, highWater, err := store.EventBounds(context.Background())
	if err != nil || minimum != 1 || highWater != 4 {
		t.Fatalf("EventBounds = (%d, %d, %v), want (1, 4, nil)", minimum, highWater, err)
	}
	events, err := store.ReplayEvents(context.Background(), 1, 0, highWater, 2)
	if err != nil {
		t.Fatalf("ReplayEvents: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 4 ||
		events[0].ConversationID != 1 || string(events[0].Payload) != `{"body":"one"}` {
		t.Fatalf("authorized events = %#v", events)
	}
	if _, err := db.Exec(`DELETE FROM conversation_members WHERE user_id = 1`); err != nil {
		t.Fatalf("remove membership: %v", err)
	}
	events, err = store.ReplayEvents(context.Background(), 1, 0, highWater, 10)
	if err != nil || len(events) != 0 {
		t.Fatalf("replay after membership removal = (%#v, %v)", events, err)
	}
}

func TestReplayStoreOrderedEventsCarryInternalMembershipChanges(t *testing.T) {
	db := openTestDB(t)
	seedReplayFixtures(t, db)
	_, err := db.Exec(`
		INSERT INTO events(seq, conversation_id, actor_id, kind, entity_id, payload, created_at) VALUES
			(5, 1, 2, 'conversation.member_removed', 1, '[1,1]', 2),
			(6, 2, 2, 'conversation.created', 2, '["private","other-room"]', 2);
	`)
	if err != nil {
		t.Fatalf("seed audience events: %v", err)
	}
	events, err := NewReplayStore(db).OrderedEvents(context.Background(), 4, 2)
	if err != nil {
		t.Fatalf("OrderedEvents: %v", err)
	}
	if len(events) != 2 || events[0].Seq != 5 || events[1].Seq != 6 {
		t.Fatalf("ordered events = %#v", events)
	}
	removed := events[0].MembershipChanges
	created := events[1].MembershipChanges
	if len(removed) != 1 || removed[0].UserID != 1 || removed[0].Joined ||
		len(created) != 1 || created[0].UserID != 2 || !created[0].Joined {
		t.Fatalf("membership changes = removed %#v created %#v", removed, created)
	}
}

func TestReplayStoreRejectsUnboundedOrInvalidPages(t *testing.T) {
	store := NewReplayStore(openTestDB(t))
	for _, query := range []struct {
		user, after, through int64
		limit                int
	}{
		{user: 0, through: 1, limit: 1},
		{user: 1, after: -1, through: 1, limit: 1},
		{user: 1, after: 2, through: 1, limit: 1},
		{user: 1, through: 1, limit: 0},
		{user: 1, through: 1, limit: 501},
	} {
		if _, err := store.ReplayEvents(
			context.Background(), query.user, query.after, query.through, query.limit,
		); err == nil {
			t.Fatalf("ReplayEvents(%#v) error = nil, want validation failure", query)
		}
	}
}

func seedReplayFixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO users(id, username, password_hash, is_admin, created_at) VALUES
			(1, 'member', 'hash', 0, 1),
			(2, 'outsider', 'hash', 0, 1);
		INSERT INTO conversations(
			id, kind, name, created_by, idempotency_key, created_at
		) VALUES
			(1, 'private', 'member-room', 1, 'room-1', 1),
			(2, 'private', 'other-room', 2, 'room-2', 1);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (1, 1, 1), (2, 2, 1);
		INSERT INTO events(seq, conversation_id, kind, entity_id, payload, created_at) VALUES
			(1, 1, 'message.sent', 11, '{"body":"one"}', 1),
			(2, 2, 'message.sent', 12, '{"body":"private"}', 1),
			(3, NULL, 'system.internal', 13, '{}', 1),
			(4, 1, 'message.edited', 11, '{"body":"four"}', 1);
	`)
	if err != nil {
		t.Fatalf("seed replay fixtures: %v", err)
	}
}
