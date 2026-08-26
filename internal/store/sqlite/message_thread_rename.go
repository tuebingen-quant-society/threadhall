package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
)

func (s *MessageStore) RenameThread(ctx context.Context, actorID, conversationID, rootID int64, title, key string, at time.Time) (message.ThreadRenameResult, error) {
	fingerprint := mustFingerprint(conversationID, rootID, title)
	result := message.ThreadRenameResult{Title: title}
	err := s.writeMessage(ctx, func(tx *sql.Tx) error {
		var storedFingerprint string
		err := tx.QueryRowContext(ctx, `SELECT fingerprint FROM thread_title_mutations WHERE actor_id = ? AND idempotency_key = ?`, actorID, key).Scan(&storedFingerprint)
		if err == nil {
			if storedFingerprint != fingerprint {
				return message.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var authorID, ownerID int64
		var admin bool
		err = tx.QueryRowContext(ctx, `SELECT root.author_id, c.created_by, actor.is_admin FROM messages root
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO thread_titles(root_message_id, conversation_id, title, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT(root_message_id) DO UPDATE SET title = excluded.title, updated_at = excluded.updated_at`, rootID, conversationID, title, unix(at)); err != nil {
			return mapMessageConstraint(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO thread_title_mutations(actor_id, idempotency_key, fingerprint, root_message_id, created_at)
			VALUES (?, ?, ?, ?, ?)`, actorID, key, fingerprint, rootID, unix(at)); err != nil {
			return mapMessageConstraint(err)
		}
		payload, _ := json.Marshal(map[string]string{"title": title})
		insert, err := tx.ExecContext(ctx, `INSERT INTO events(conversation_id, actor_id, kind, entity_id, payload, created_at)
			VALUES (?, ?, 'thread.renamed', ?, ?, ?)`, conversationID, actorID, rootID, payload, unix(at))
		if err != nil {
			return err
		}
		seq, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		result.Event = realtime.Event{Seq: seq, Type: "thread.renamed", ConversationID: conversationID, EntityID: rootID, Payload: payload}
		return nil
	})
	return result, err
}
