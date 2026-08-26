package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func (s *MessageStore) Thread(ctx context.Context, query message.Thread) (message.ThreadPage, error) {
	root, err := scanMessage(s.db.QueryRowContext(ctx, `SELECT
		m.id, m.conversation_id, m.author_id, m.thread_root_id, m.reference_message_id, m.body, m.rendered_body,
		m.created_at, m.edited_at, m.deleted_at
		FROM messages m JOIN conversation_members member
			ON member.conversation_id = m.conversation_id AND member.user_id = ?
		WHERE m.id = ? AND m.conversation_id = ? AND m.reply_to_id IS NULL AND m.thread_root_id IS NULL`,
		query.UserID, query.RootMessageID, query.ConversationID))
	if errors.Is(err, sql.ErrNoRows) {
		return message.ThreadPage{}, message.ErrNotFound
	}
	if err != nil {
		return message.ThreadPage{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT
		m.id, m.conversation_id, m.author_id, m.thread_root_id, m.reference_message_id, m.body, m.rendered_body,
		m.created_at, m.edited_at, m.deleted_at
		FROM messages m JOIN conversation_members member
			ON member.conversation_id = m.conversation_id AND member.user_id = ?
		WHERE m.thread_root_id = ? AND m.conversation_id = ? AND (? = 0 OR m.id > ?)
		ORDER BY m.id ASC LIMIT ?`, query.UserID, query.RootMessageID, query.ConversationID,
		query.AfterID, query.AfterID, query.Limit+1)
	if err != nil {
		return message.ThreadPage{}, err
	}
	defer rows.Close()
	page := message.ThreadPage{Root: root, Replies: make([]message.Message, 0, query.Limit)}
	for rows.Next() {
		item, err := scanMessage(rows)
		if err != nil {
			return message.ThreadPage{}, err
		}
		page.Replies = append(page.Replies, item)
	}
	if err := rows.Err(); err != nil {
		return message.ThreadPage{}, err
	}
	if len(page.Replies) > query.Limit {
		page.Replies = page.Replies[:query.Limit]
		page.NextAfterID = page.Replies[query.Limit-1].ID
	}
	all := make([]message.Message, 0, 1+len(page.Replies))
	all = append(all, page.Root)
	all = append(all, page.Replies...)
	if err := s.attachInlineApps(ctx, all); err != nil {
		return message.ThreadPage{}, err
	}
	if err := s.attachQuestions(ctx, all); err != nil {
		return message.ThreadPage{}, err
	}
	page.Root, page.Replies = all[0], all[1:]
	return page, nil
}
