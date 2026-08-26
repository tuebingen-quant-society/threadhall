package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func (s *ConversationStore) Fork(ctx context.Context, record conversation.ForkRecord) (conversation.Fork, error) {
	fingerprint := mustFingerprint(record.SourceConversationID, record.SourceMessageID, record.Kind, record.Name)
	var fork conversation.Fork
	err := s.write(ctx, func(tx *sql.Tx) error {
		conversationID, found, err := findForkMutation(ctx, tx, record.ActorID, record.IdempotencyKey, fingerprint)
		if err != nil {
			return err
		}
		if found {
			fork, err = forkByConversationID(ctx, tx, conversationID)
			return err
		}
		var rootID int64
		err = tx.QueryRowContext(ctx, `SELECT COALESCE(m.thread_root_id, m.id)
			FROM messages m JOIN conversation_members member
				ON member.conversation_id = m.conversation_id AND member.user_id = ?
			WHERE m.id = ? AND m.conversation_id = ? AND m.reply_to_id IS NULL`,
			record.ActorID, record.SourceMessageID, record.SourceConversationID).Scan(&rootID)
		if errors.Is(err, sql.ErrNoRows) {
			return conversation.ErrNotFound
		}
		if err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO conversations(
			kind, name, created_by, idempotency_key, created_at) VALUES (?, ?, ?, ?, ?)`,
			record.Kind, record.Name, record.ActorID, forkStorageKey(record.ActorID, record.IdempotencyKey), unix(record.CreatedAt))
		if err != nil {
			return mapConversationConstraint(err)
		}
		conversationID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (?, ?, ?)`, conversationID, record.ActorID, unix(record.CreatedAt)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_forks(
			conversation_id, source_conversation_id, source_root_message_id, created_by, created_at)
			VALUES (?, ?, ?, ?, ?)`, conversationID, record.SourceConversationID, rootID,
			record.ActorID, unix(record.CreatedAt)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_fork_mutations(
			actor_id, idempotency_key, fingerprint, conversation_id, created_at) VALUES (?, ?, ?, ?, ?)`,
			record.ActorID, record.IdempotencyKey, fingerprint, conversationID, unix(record.CreatedAt)); err != nil {
			return mapConversationConstraint(err)
		}
		fork = conversation.Fork{
			Conversation:         conversation.Conversation{ID: conversationID, Kind: record.Kind, Name: record.Name, CreatedBy: record.ActorID, CreatedAt: record.CreatedAt.UTC()},
			SourceConversationID: record.SourceConversationID, SourceRootMessageID: rootID,
		}
		return insertConversationEvent(ctx, tx, conversationID, record.ActorID, "conversation.forked",
			mustFingerprint(record.SourceConversationID, rootID), unix(record.CreatedAt))
	})
	return fork, err
}

func findForkMutation(ctx context.Context, tx *sql.Tx, actorID int64, key, fingerprint string) (int64, bool, error) {
	var stored string
	var conversationID int64
	err := tx.QueryRowContext(ctx, `SELECT fingerprint, conversation_id FROM conversation_fork_mutations
		WHERE actor_id = ? AND idempotency_key = ?`, actorID, key).Scan(&stored, &conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if stored != fingerprint {
		return 0, false, conversation.ErrConflict
	}
	return conversationID, true, nil
}

func forkByConversationID(ctx context.Context, tx *sql.Tx, id int64) (conversation.Fork, error) {
	fork := conversation.Fork{}
	var createdAt int64
	err := tx.QueryRowContext(ctx, `SELECT c.id, c.kind, c.name, c.created_by, c.created_at,
		f.source_conversation_id, f.source_root_message_id
		FROM conversations c JOIN conversation_forks f ON f.conversation_id = c.id WHERE c.id = ?`, id).Scan(
		&fork.Conversation.ID, &fork.Conversation.Kind, &fork.Conversation.Name, &fork.Conversation.CreatedBy,
		&createdAt, &fork.SourceConversationID, &fork.SourceRootMessageID)
	fork.Conversation.CreatedAt = time.Unix(createdAt, 0).UTC()
	return fork, err
}

func forkStorageKey(actorID int64, key string) string {
	digest := sha256.Sum256([]byte(mustFingerprint("fork", actorID, key)))
	return "fork:" + hex.EncodeToString(digest[:])
}
