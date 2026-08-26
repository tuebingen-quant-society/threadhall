package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func (s *MessageStore) Edit(ctx context.Context, record message.EditRecord) (message.Result, error) {
	fingerprint := messageFingerprint(record.MessageID, record.Body)
	var result message.Result
	err := s.writeMessage(ctx, func(tx *sql.Tx) error {
		stored, found, err := findMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "edit", fingerprint)
		if err != nil {
			return err
		}
		if found {
			result = stored
			return nil
		}
		if err := rejectLegacyMessageKey(ctx, tx, record.AuthorID, record.IdempotencyKey); err != nil {
			return err
		}
		item, err := editableMessage(ctx, tx, record.MessageID, record.AuthorID)
		if err != nil {
			return err
		}
		update, err := tx.ExecContext(ctx, `UPDATE messages
			SET body = ?, rendered_body = ?, edited_at = ?
			WHERE id = ? AND author_id = ? AND reply_to_id IS NULL AND deleted_at IS NULL`,
			record.Body, record.RenderedBody, unix(record.EditedAt), record.MessageID, record.AuthorID)
		if err != nil {
			return err
		}
		changed, err := update.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return message.ErrNotFound
		}
		editedAt := record.EditedAt.UTC()
		item.Body, item.RenderedBody, item.EditedAt = record.Body, record.RenderedBody, &editedAt
		event, err := insertMessageEvent(ctx, tx, "message.edited", item, record.EditedAt)
		if err != nil {
			return err
		}
		result = message.Result{Message: item, Event: event}
		return recordMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "edit",
			fingerprint, result, record.EditedAt)
	})
	return result, err
}

func (s *MessageStore) Delete(ctx context.Context, record message.DeleteRecord) (message.Result, error) {
	fingerprint := messageFingerprint(record.MessageID)
	var result message.Result
	err := s.writeMessage(ctx, func(tx *sql.Tx) error {
		stored, found, err := findMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "delete", fingerprint)
		if err != nil {
			return err
		}
		if found {
			result = stored
			return nil
		}
		if err := rejectLegacyMessageKey(ctx, tx, record.AuthorID, record.IdempotencyKey); err != nil {
			return err
		}
		item, err := editableMessage(ctx, tx, record.MessageID, record.AuthorID)
		if err != nil {
			return err
		}
		update, err := tx.ExecContext(ctx, `UPDATE messages
			SET body = '', rendered_body = '', deleted_at = ?
			WHERE id = ? AND author_id = ? AND reply_to_id IS NULL AND deleted_at IS NULL`,
			unix(record.DeletedAt), record.MessageID, record.AuthorID)
		if err != nil {
			return err
		}
		changed, err := update.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return message.ErrNotFound
		}
		deletedAt := record.DeletedAt.UTC()
		item.Body, item.RenderedBody, item.DeletedAt = "", "", &deletedAt
		event, err := insertMessageEvent(ctx, tx, "message.deleted", item, record.DeletedAt)
		if err != nil {
			return err
		}
		result = message.Result{Message: item, Event: event}
		return recordMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "delete",
			fingerprint, result, record.DeletedAt)
	})
	return result, err
}

func editableMessage(ctx context.Context, tx *sql.Tx, messageID, authorID int64) (message.Message, error) {
	item, err := scanMessage(tx.QueryRowContext(ctx, `SELECT
		m.id, m.conversation_id, m.author_id, m.thread_root_id, m.body, m.rendered_body,
		m.created_at, m.edited_at, m.deleted_at
		FROM messages m
		JOIN conversation_members member
			ON member.conversation_id = m.conversation_id AND member.user_id = ?
		WHERE m.id = ? AND m.author_id = ? AND m.reply_to_id IS NULL
			AND m.deleted_at IS NULL`, authorID, messageID, authorID))
	if errors.Is(err, sql.ErrNoRows) {
		return message.Message{}, message.ErrNotFound
	}
	return item, err
}
