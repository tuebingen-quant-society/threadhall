package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func (s *MessageStore) MarkThreadRead(ctx context.Context, userID, conversationID, rootID int64, at time.Time) error {
	return s.writeMessage(ctx, func(tx *sql.Tx) error {
		var allowed bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM messages root JOIN conversation_members member
			ON member.conversation_id = root.conversation_id AND member.user_id = ?
			WHERE root.id = ? AND root.conversation_id = ? AND root.thread_root_id IS NULL)`, userID, rootID, conversationID).Scan(&allowed); err != nil {
			return err
		}
		if !allowed {
			return message.ErrNotFound
		}
		var latest int64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(id), ?) FROM messages WHERE thread_root_id = ?`, rootID, rootID).Scan(&latest); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO thread_reads(user_id, root_message_id, last_read_message_id, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(user_id, root_message_id) DO UPDATE SET
			last_read_message_id = max(last_read_message_id, excluded.last_read_message_id), updated_at = excluded.updated_at`, userID, rootID, latest, unix(at))
		return err
	})
}

func (s *MessageStore) DeleteThread(ctx context.Context, actorID, conversationID, rootID int64) error {
	return s.writeMessage(ctx, func(tx *sql.Tx) error {
		var authorID, ownerID int64
		var admin bool
		err := tx.QueryRowContext(ctx, `SELECT root.author_id, c.created_by, actor.is_admin FROM messages root
			JOIN conversations c ON c.id = root.conversation_id JOIN users actor ON actor.id = ? AND actor.principal_kind = 'human'
			WHERE root.id = ? AND root.conversation_id = ? AND root.thread_root_id IS NULL`, actorID, rootID, conversationID).Scan(&authorID, &ownerID, &admin)
		if errors.Is(err, sql.ErrNoRows) {
			return message.ErrNotFound
		}
		if err != nil {
			return err
		}
		if actorID != authorID && actorID != ownerID && !admin {
			return message.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM conversation_forks WHERE source_root_message_id = ?`, rootID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_tasks WHERE thread_root_id = ? OR invoking_message_id = ? OR output_message_id = ?`, rootID, rootID, rootID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_tasks WHERE invoking_message_id IN (SELECT id FROM messages WHERE thread_root_id = ?) OR output_message_id IN (SELECT id FROM messages WHERE thread_root_id = ?)`, rootID, rootID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE thread_root_id = ?`, rootID); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE id = ?`, rootID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return message.ErrNotFound
		}
		return nil
	})
}
