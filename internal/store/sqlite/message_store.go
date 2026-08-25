package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/mattn/go-sqlite3"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
)

// MessageStore implements message.Repository over the durable core schema.
type MessageStore struct {
	db     *sql.DB
	writer *Writer
}

func NewMessageStore(db *sql.DB, writer *Writer) *MessageStore {
	return &MessageStore{db: db, writer: writer}
}

func (s *MessageStore) Send(ctx context.Context, record message.SendRecord) (message.Result, error) {
	fingerprint := messageFingerprint(record.ConversationID, record.Body)
	var result message.Result
	err := s.writeMessage(ctx, func(tx *sql.Tx) error {
		stored, found, err := findMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "send", fingerprint)
		if err != nil {
			return err
		}
		if found {
			result = stored
			return nil
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO messages(
			conversation_id, author_id, body, rendered_body, idempotency_key, created_at)
			SELECT conversation_id, ?, ?, ?, ?, ? FROM conversation_members
			WHERE conversation_id = ? AND user_id = ?`,
			record.AuthorID, record.Body, record.RenderedBody, record.IdempotencyKey,
			unix(record.CreatedAt), record.ConversationID, record.AuthorID)
		if err != nil {
			return mapMessageConstraint(err)
		}
		changed, err := insert.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return message.ErrNotFound
		}
		id, err := insert.LastInsertId()
		if err != nil {
			return err
		}
		item := message.Message{
			ID: id, ConversationID: record.ConversationID, AuthorID: record.AuthorID,
			Body: record.Body, RenderedBody: record.RenderedBody, CreatedAt: record.CreatedAt.UTC(),
		}
		event, err := insertMessageEvent(ctx, tx, "message.sent", item, record.CreatedAt)
		if err != nil {
			return err
		}
		result = message.Result{Message: item, Event: event}
		return recordMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "send",
			fingerprint, result, record.CreatedAt)
	})
	return result, err
}

func findMessageMutation(
	ctx context.Context,
	tx *sql.Tx,
	actorID int64,
	key, operation, fingerprint string,
) (message.Result, bool, error) {
	var storedOperation, storedFingerprint string
	var encoded []byte
	err := tx.QueryRowContext(ctx, `SELECT operation, fingerprint, result_json
		FROM message_mutations WHERE actor_id = ? AND idempotency_key = ?`, actorID, key).
		Scan(&storedOperation, &storedFingerprint, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return message.Result{}, false, nil
	}
	if err != nil {
		return message.Result{}, false, err
	}
	if storedOperation != operation || storedFingerprint != fingerprint {
		return message.Result{}, false, message.ErrConflict
	}
	var result message.Result
	if err := json.Unmarshal(encoded, &result); err != nil {
		return message.Result{}, false, err
	}
	return result, true, nil
}

func recordMessageMutation(
	ctx context.Context,
	tx *sql.Tx,
	actorID int64,
	key, operation, fingerprint string,
	result message.Result,
	createdAt time.Time,
) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO message_mutations(
		actor_id, idempotency_key, operation, fingerprint, message_id, result_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, actorID, key, operation, fingerprint,
		result.Message.ID, encoded, unix(createdAt))
	return mapMessageConstraint(err)
}

func insertMessageEvent(
	ctx context.Context,
	tx *sql.Tx,
	kind string,
	item message.Message,
	createdAt time.Time,
) (realtime.Event, error) {
	payload, err := messageEventPayload(kind, item)
	if err != nil {
		return realtime.Event{}, err
	}
	insert, err := tx.ExecContext(ctx, `INSERT INTO events(
		conversation_id, actor_id, kind, entity_id, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, item.ConversationID, item.AuthorID, kind, item.ID, payload, unix(createdAt))
	if err != nil {
		return realtime.Event{}, err
	}
	seq, err := insert.LastInsertId()
	if err != nil {
		return realtime.Event{}, err
	}
	return realtime.Event{
		Seq: seq, Type: kind, ConversationID: item.ConversationID,
		EntityID: item.ID, Payload: payload,
	}, nil
}

func messageEventPayload(kind string, item message.Message) (json.RawMessage, error) {
	switch kind {
	case "message.sent":
		return json.Marshal(struct {
			AuthorID     int64     `json:"author_id"`
			Body         string    `json:"body"`
			RenderedBody string    `json:"rendered_body"`
			CreatedAt    time.Time `json:"created_at"`
		}{item.AuthorID, item.Body, item.RenderedBody, item.CreatedAt})
	case "message.edited":
		return json.Marshal(struct {
			Body         string    `json:"body"`
			RenderedBody string    `json:"rendered_body"`
			EditedAt     time.Time `json:"edited_at"`
		}{item.Body, item.RenderedBody, *item.EditedAt})
	case "message.deleted":
		return json.Marshal(struct {
			DeletedAt time.Time `json:"deleted_at"`
		}{*item.DeletedAt})
	default:
		return nil, errors.New("unknown message event type")
	}
}

func messageFingerprint(values ...any) string {
	encoded, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func mapMessageConstraint(err error) error {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
		return message.ErrConflict
	}
	return err
}

func (s *MessageStore) writeMessage(ctx context.Context, fn WriteFunc) error {
	err := s.writer.Do(ctx, fn)
	if errors.Is(err, ErrBusy) {
		return message.ErrBusy
	}
	return err
}
