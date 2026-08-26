package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
)

// ReplayStore reads global event bounds and membership-authorized event pages.
type ReplayStore struct{ db *sql.DB }

func NewReplayStore(db *sql.DB) *ReplayStore { return &ReplayStore{db: db} }

func (s *ReplayStore) Memberships(ctx context.Context, userID int64) ([]int64, error) {
	if s == nil || s.db == nil || userID <= 0 {
		return nil, errors.New("invalid membership snapshot")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT conversation_id
		FROM conversation_members WHERE user_id = ?
		ORDER BY conversation_id LIMIT ?`, userID, realtime.MaxSubscriberMemberships+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	conversationIDs := make([]int64, 0)
	for rows.Next() {
		var conversationID int64
		if err := rows.Scan(&conversationID); err != nil {
			return nil, err
		}
		conversationIDs = append(conversationIDs, conversationID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(conversationIDs) > realtime.MaxSubscriberMemberships {
		return nil, errors.New("membership snapshot exceeds limit")
	}
	return conversationIDs, nil
}

func (s *ReplayStore) EventBounds(ctx context.Context) (int64, int64, error) {
	if s == nil || s.db == nil {
		return 0, 0, errors.New("replay database is required")
	}
	var minimum, highWater int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MIN(seq), 0), COALESCE(MAX(seq), 0) FROM events`,
	).Scan(&minimum, &highWater)
	return minimum, highWater, err
}

func (s *ReplayStore) ReplayEvents(
	ctx context.Context,
	userID, afterSeq, throughSeq int64,
	limit int,
) ([]realtime.Event, error) {
	if s == nil || s.db == nil || userID <= 0 || afterSeq < 0 ||
		throughSeq < afterSeq || limit <= 0 || limit > realtime.ReplayPageSize {
		return nil, errors.New("invalid replay page")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		e.seq, e.kind, e.conversation_id, e.entity_id, e.payload
		FROM events e
		JOIN conversation_members member
			ON member.conversation_id = e.conversation_id AND member.user_id = ?
		WHERE e.conversation_id IS NOT NULL AND e.conversation_id > 0
			AND e.seq > ? AND e.seq <= ?
		ORDER BY e.seq ASC LIMIT ?`, userID, afterSeq, throughSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]realtime.Event, 0, limit)
	for rows.Next() {
		var event realtime.Event
		var entityID sql.NullInt64
		var payload []byte
		if err := rows.Scan(
			&event.Seq, &event.Type, &event.ConversationID, &entityID, &payload,
		); err != nil {
			return nil, err
		}
		if entityID.Valid {
			event.EntityID = entityID.Int64
		}
		event.Payload = append(event.Payload[:0], payload...)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *ReplayStore) OrderedEvents(
	ctx context.Context,
	afterSeq int64,
	limit int,
) ([]realtime.Event, error) {
	if s == nil || s.db == nil || afterSeq < 0 || limit <= 0 || limit > realtime.PumpPageSize {
		return nil, errors.New("invalid ordered event page")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		seq, kind, conversation_id, actor_id, entity_id, payload
		FROM events WHERE seq > ? ORDER BY seq ASC LIMIT ?`, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]realtime.Event, 0, limit)
	for rows.Next() {
		var event realtime.Event
		var conversationID, actorID, entityID sql.NullInt64
		var payload []byte
		if err := rows.Scan(
			&event.Seq, &event.Type, &conversationID, &actorID, &entityID, &payload,
		); err != nil {
			return nil, err
		}
		event.ConversationID = conversationID.Int64
		event.EntityID = entityID.Int64
		event.Payload = append(event.Payload[:0], payload...)
		changes, err := eventMembershipChanges(event, actorID.Int64)
		if err != nil {
			return nil, err
		}
		event.MembershipChanges = changes
		events = append(events, event)
	}
	return events, rows.Err()
}

func eventMembershipChanges(event realtime.Event, actorID int64) ([]realtime.MembershipChange, error) {
	switch event.Type {
	case "conversation.member_added", "conversation.member_removed":
		var identifiers []int64
		if err := json.Unmarshal(event.Payload, &identifiers); err != nil || len(identifiers) != 2 ||
			identifiers[0] != event.ConversationID || identifiers[1] <= 0 {
			return nil, fmt.Errorf("invalid durable membership event")
		}
		return []realtime.MembershipChange{{
			UserID: identifiers[1], Joined: event.Type == "conversation.member_added",
		}}, nil
	case "conversation.created":
		var dmUsers []int64
		if err := json.Unmarshal(event.Payload, &dmUsers); err == nil && len(dmUsers) == 2 &&
			dmUsers[0] > 0 && dmUsers[1] > 0 {
			return []realtime.MembershipChange{
				{UserID: dmUsers[0], Joined: true}, {UserID: dmUsers[1], Joined: true},
			}, nil
		}
		if actorID <= 0 {
			return nil, fmt.Errorf("invalid durable conversation event")
		}
		return []realtime.MembershipChange{{UserID: actorID, Joined: true}}, nil
	case "conversation.forked":
		if actorID <= 0 {
			return nil, fmt.Errorf("invalid durable fork event")
		}
		return []realtime.MembershipChange{{UserID: actorID, Joined: true}}, nil
	default:
		return nil, nil
	}
}
