package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func legacyConversationByKey(ctx context.Context, tx *sql.Tx, actorID int64, key string) (conversation.Conversation, error) {
	return scanConversation(tx.QueryRowContext(ctx, `SELECT id, kind, name, created_by, created_at
		FROM conversations WHERE created_by = ? AND idempotency_key = ?`, actorID, key))
}

func legacyDMByKey(
	ctx context.Context,
	tx *sql.Tx,
	actorID int64,
	key string,
	lowID, highID int64,
) (conversation.Conversation, bool, error) {
	row := tx.QueryRowContext(ctx, `SELECT id, kind, name, created_by, created_at,
		CASE WHEN kind = 'dm' AND dm_user_low = ? AND dm_user_high = ? THEN 1 ELSE 0 END
		FROM conversations WHERE created_by = ? AND idempotency_key = ?`, lowID, highID, actorID, key)
	var result conversation.Conversation
	var name sql.NullString
	var createdAt int64
	var exact bool
	if err := row.Scan(&result.ID, &result.Kind, &name, &result.CreatedBy, &createdAt, &exact); err != nil {
		return conversation.Conversation{}, false, err
	}
	result.Name, result.CreatedAt = name.String, time.Unix(createdAt, 0).UTC()
	return result, exact, nil
}

func rejectLegacyIdempotency(ctx context.Context, tx *sql.Tx, actorID int64, key string) error {
	_, err := legacyConversationByKey(ctx, tx, actorID, key)
	if err == nil {
		return conversation.ErrConflict
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}
