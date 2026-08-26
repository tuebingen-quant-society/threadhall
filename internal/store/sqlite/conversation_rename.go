package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func (s *ConversationStore) RenameConversation(ctx context.Context, record conversation.RenameRecord) (conversation.Conversation, error) {
	fingerprint := mustFingerprint(record.ConversationID, record.Name)
	var renamed conversation.Conversation
	err := s.write(ctx, func(tx *sql.Tx) error {
		var storedFingerprint string
		err := tx.QueryRowContext(ctx, `SELECT fingerprint FROM conversation_rename_mutations WHERE actor_id = ? AND idempotency_key = ?`, record.ActorID, record.IdempotencyKey).Scan(&storedFingerprint)
		if err == nil {
			if storedFingerprint != fingerprint {
				return conversation.ErrConflict
			}
			renamed, err = conversationByID(ctx, tx, record.ConversationID)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var owner int64
		var kind conversation.Kind
		var admin bool
		err = tx.QueryRowContext(ctx, `SELECT c.created_by, c.kind, actor.is_admin FROM conversations c
			JOIN users actor ON actor.id = ? AND actor.principal_kind = 'human' WHERE c.id = ?`, record.ActorID, record.ConversationID).Scan(&owner, &kind, &admin)
		if errors.Is(err, sql.ErrNoRows) || kind == conversation.KindDM {
			return conversation.ErrNotFound
		}
		if err != nil {
			return err
		}
		if record.ActorID != owner && !admin {
			return conversation.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `UPDATE conversations SET name = ? WHERE id = ?`, record.Name, record.ConversationID); err != nil {
			return mapConversationConstraint(err)
		}
		renamed, err = conversationByID(ctx, tx, record.ConversationID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_rename_mutations(actor_id, idempotency_key, fingerprint, conversation_id, created_at) VALUES (?, ?, ?, ?, ?)`, record.ActorID, record.IdempotencyKey, fingerprint, record.ConversationID, unix(record.RenamedAt)); err != nil {
			return mapConversationConstraint(err)
		}
		return insertConversationEvent(ctx, tx, record.ConversationID, record.ActorID, "conversation.renamed", fingerprint, unix(record.RenamedAt))
	})
	return renamed, err
}
