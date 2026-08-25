package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func (s *MessageStore) History(ctx context.Context, query message.History) (message.Page, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
		m.id, m.conversation_id, m.author_id, m.body, m.rendered_body,
		m.created_at, m.edited_at, m.deleted_at
		FROM messages m
		JOIN conversation_members member
			ON member.conversation_id = m.conversation_id AND member.user_id = ?
		WHERE m.conversation_id = ? AND (? = 0 OR m.id < ?)
		ORDER BY m.id DESC LIMIT ?`, query.UserID, query.ConversationID,
		query.BeforeID, query.BeforeID, query.Limit+1)
	if err != nil {
		return message.Page{}, err
	}
	defer rows.Close()
	page := message.Page{Messages: make([]message.Message, 0, query.Limit)}
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return message.Page{}, err
		}
		page.Messages = append(page.Messages, item)
	}
	if err := rows.Err(); err != nil {
		return message.Page{}, err
	}
	if len(page.Messages) == 0 {
		allowed, err := s.canReadConversation(ctx, query.UserID, query.ConversationID)
		if err != nil {
			return message.Page{}, err
		}
		if !allowed {
			return message.Page{}, message.ErrNotFound
		}
	}
	if len(page.Messages) > query.Limit {
		page.Messages = page.Messages[:query.Limit]
		page.NextBeforeID = page.Messages[query.Limit-1].ID
	}
	return page, nil
}

func (s *MessageStore) canReadConversation(ctx context.Context, userID, conversationID int64) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM conversations c
		JOIN conversation_members member ON member.conversation_id = c.id
		WHERE c.id = ? AND member.user_id = ?)`, conversationID, userID).Scan(&allowed)
	return allowed, err
}

func scanMessage(row rowScanner) (message.Message, error) {
	var item message.Message
	var createdAt int64
	var editedAt, deletedAt sql.NullInt64
	err := row.Scan(&item.ID, &item.ConversationID, &item.AuthorID, &item.Body,
		&item.RenderedBody, &createdAt, &editedAt, &deletedAt)
	if err != nil {
		return message.Message{}, err
	}
	item.CreatedAt = time.Unix(createdAt, 0).UTC()
	if editedAt.Valid {
		value := time.Unix(editedAt.Int64, 0).UTC()
		item.EditedAt = &value
	}
	if deletedAt.Valid {
		value := time.Unix(deletedAt.Int64, 0).UTC()
		item.DeletedAt = &value
	}
	return item, nil
}
