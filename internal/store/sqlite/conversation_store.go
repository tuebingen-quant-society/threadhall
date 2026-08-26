package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

// ConversationStore implements conversation.Repository over the core schema.
type ConversationStore struct {
	db     *sql.DB
	writer *Writer
}

func NewConversationStore(db *sql.DB, writer *Writer) *ConversationStore {
	return &ConversationStore{db: db, writer: writer}
}

func (s *ConversationStore) List(ctx context.Context, userID, beforeID int64, limit int) (conversation.ConversationPage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.kind, c.name, c.created_by, c.created_at, peer.username
		FROM conversations c
		JOIN conversation_members member ON member.conversation_id = c.id
		LEFT JOIN users peer ON c.kind = 'dm' AND peer.id = CASE
			WHEN c.dm_user_low = ? THEN c.dm_user_high ELSE c.dm_user_low END
		WHERE member.user_id = ? AND (? = 0 OR c.id < ?)
		ORDER BY c.id DESC LIMIT ?`, userID, userID, beforeID, beforeID, limit+1)
	if err != nil {
		return conversation.ConversationPage{}, err
	}
	defer rows.Close()
	page := conversation.ConversationPage{Conversations: make([]conversation.Conversation, 0, limit)}
	for rows.Next() {
		item, err := scanUserConversation(rows)
		if err != nil {
			return conversation.ConversationPage{}, err
		}
		page.Conversations = append(page.Conversations, item)
	}
	if err := rows.Err(); err != nil {
		return conversation.ConversationPage{}, err
	}
	if len(page.Conversations) > limit {
		page.Conversations = page.Conversations[:limit]
		page.NextBeforeID = page.Conversations[limit-1].ID
	}
	return page, nil
}

func (s *ConversationStore) Detail(ctx context.Context, userID, conversationID int64) (conversation.Conversation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.kind, c.name, c.created_by, c.created_at, peer.username
		FROM conversations c
		JOIN conversation_members member ON member.conversation_id = c.id
		LEFT JOIN users peer ON c.kind = 'dm' AND peer.id = CASE
			WHEN c.dm_user_low = ? THEN c.dm_user_high ELSE c.dm_user_low END
		WHERE c.id = ? AND member.user_id = ?`, userID, conversationID, userID)
	item, err := scanUserConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return conversation.Conversation{}, conversation.ErrNotFound
	}
	return item, err
}

func (s *ConversationStore) ListMembers(ctx context.Context, userID, conversationID, beforeID int64, limit int) (conversation.MemberPage, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT users.id, users.username, users.principal_kind, member.joined_at
		FROM conversation_members member JOIN users ON users.id = member.user_id
		WHERE member.conversation_id = ? AND (? = 0 OR users.id < ?)
		AND EXISTS(SELECT 1 FROM conversation_members caller
			WHERE caller.conversation_id = member.conversation_id AND caller.user_id = ?)
		ORDER BY users.id DESC LIMIT ?`, conversationID, beforeID, beforeID, userID, limit+1)
	if err != nil {
		return conversation.MemberPage{}, err
	}
	defer rows.Close()
	page := conversation.MemberPage{Members: make([]conversation.Member, 0, limit)}
	for rows.Next() {
		var member conversation.Member
		var joinedAt int64
		if err := rows.Scan(&member.UserID, &member.Username, &member.PrincipalKind, &joinedAt); err != nil {
			return conversation.MemberPage{}, err
		}
		member.JoinedAt = time.Unix(joinedAt, 0).UTC()
		page.Members = append(page.Members, member)
	}
	if err := rows.Err(); err != nil {
		return conversation.MemberPage{}, err
	}
	if len(page.Members) == 0 {
		allowed, err := s.CanRead(ctx, userID, conversationID)
		if err != nil {
			return conversation.MemberPage{}, err
		}
		if !allowed {
			return conversation.MemberPage{}, conversation.ErrNotFound
		}
	}
	if len(page.Members) > limit {
		page.Members = page.Members[:limit]
		page.NextBeforeID = page.Members[limit-1].UserID
	}
	return page, nil
}

func (s *ConversationStore) CanRead(ctx context.Context, userID, conversationID int64) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM conversation_members
			WHERE conversation_id = ? AND user_id = ?
		)`, conversationID, userID).Scan(&allowed)
	return allowed, err
}

type rowScanner interface{ Scan(...any) error }

func scanConversation(row rowScanner) (conversation.Conversation, error) {
	var item conversation.Conversation
	var name sql.NullString
	var createdAt int64
	err := row.Scan(&item.ID, &item.Kind, &name, &item.CreatedBy, &createdAt)
	if err != nil {
		return conversation.Conversation{}, err
	}
	item.Name = name.String
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	return item, nil
}

func scanUserConversation(row rowScanner) (conversation.Conversation, error) {
	var item conversation.Conversation
	var name, peer sql.NullString
	var createdAt int64
	err := row.Scan(&item.ID, &item.Kind, &name, &item.CreatedBy, &createdAt, &peer)
	if err != nil {
		return conversation.Conversation{}, err
	}
	item.Name, item.PeerUsername = name.String, peer.String
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	return item, nil
}
