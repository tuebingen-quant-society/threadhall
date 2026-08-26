package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
)

func adoptLegacySend(
	ctx context.Context,
	tx *sql.Tx,
	record message.SendRecord,
	fingerprint string,
) (message.Result, bool, error) {
	var id, conversationID, authorID int64
	var replyToID, threadRootID sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT id, conversation_id, author_id, reply_to_id, thread_root_id
		FROM messages WHERE author_id = ? AND idempotency_key = ?`,
		record.AuthorID, record.IdempotencyKey).Scan(&id, &conversationID, &authorID, &replyToID, &threadRootID)
	if errors.Is(err, sql.ErrNoRows) {
		return message.Result{}, false, nil
	}
	if err != nil {
		return message.Result{}, false, err
	}
	if replyToID.Valid || threadRootID.Valid || record.ThreadRootID != nil || conversationID != record.ConversationID {
		return message.Result{}, false, message.ErrConflict
	}

	var event realtime.Event
	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT seq, payload FROM events
		WHERE kind = 'message.sent' AND conversation_id = ? AND actor_id = ? AND entity_id = ?
		ORDER BY seq ASC LIMIT 1`, conversationID, authorID, id).Scan(&event.Seq, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return message.Result{}, false, message.ErrConflict
	}
	if err != nil {
		return message.Result{}, false, err
	}
	var original struct {
		AuthorID     int64     `json:"author_id"`
		Body         string    `json:"body"`
		RenderedBody string    `json:"rendered_body"`
		CreatedAt    time.Time `json:"created_at"`
	}
	if err := json.Unmarshal(payload, &original); err != nil {
		return message.Result{}, false, fmt.Errorf("decode legacy send event: %w", err)
	}
	if original.AuthorID != authorID || original.Body != record.Body || original.CreatedAt.IsZero() {
		return message.Result{}, false, message.ErrConflict
	}
	item := message.Message{
		ID: id, ConversationID: conversationID, AuthorID: authorID,
		Body: original.Body, RenderedBody: original.RenderedBody, CreatedAt: original.CreatedAt.UTC(),
	}
	event.Type, event.ConversationID, event.EntityID = "message.sent", conversationID, id
	event.Payload = append(event.Payload[:0], payload...)
	result := message.Result{Message: item, Event: event}
	if err := recordMessageMutation(ctx, tx, authorID, record.IdempotencyKey, "send",
		fingerprint, result, original.CreatedAt); err != nil {
		return message.Result{}, false, err
	}
	return result, true, nil
}

func rejectLegacyMessageKey(ctx context.Context, tx *sql.Tx, actorID int64, key string) error {
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM messages WHERE author_id = ? AND idempotency_key = ?)`, actorID, key).
		Scan(&exists); err != nil {
		return err
	}
	if exists {
		return message.ErrConflict
	}
	return nil
}
