package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func (s *ConversationStore) MarkRead(ctx context.Context, userID, conversationID int64, at time.Time) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var member bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id = ? AND user_id = ?)`, conversationID, userID).Scan(&member); err != nil {
			return err
		}
		if !member {
			return conversation.ErrNotFound
		}
		var latest int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(id), 0) FROM messages WHERE conversation_id = ? AND thread_root_id IS NULL`, conversationID).Scan(&latest); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO conversation_reads(user_id, conversation_id, last_read_message_id, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(user_id, conversation_id) DO UPDATE SET
			last_read_message_id = max(last_read_message_id, excluded.last_read_message_id), updated_at = excluded.updated_at`,
			userID, conversationID, latest, unix(at))
		return err
	})
}

func (s *ConversationStore) DeleteConversation(ctx context.Context, actorID, conversationID int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var kind conversation.Kind
		var owner int64
		var admin bool
		err := tx.QueryRowContext(ctx, `SELECT c.kind, c.created_by, u.is_admin FROM conversations c
			JOIN users u ON u.id = ? AND u.principal_kind = 'human' WHERE c.id = ?`, actorID, conversationID).Scan(&kind, &owner, &admin)
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ErrNotFound
		}
		if err != nil {
			return err
		}
		if kind == conversation.KindDM {
			return conversation.ErrNotFound
		}
		if actorID != owner && !admin {
			return conversation.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM conversation_forks WHERE source_conversation_id = ?`, conversationID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_tasks WHERE conversation_id = ?`, conversationID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, conversationID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return conversation.ErrNotFound
		}
		return nil
	})
}
