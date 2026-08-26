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
	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
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
	fingerprint := messageFingerprint(record.ConversationID, record.ThreadRootID, record.ReplyToMessageID, record.Body)
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
		adopted, found, err := adoptLegacySend(ctx, tx, record, fingerprint)
		if err != nil {
			return err
		}
		if found {
			result = adopted
			return nil
		}
		insert, err := tx.ExecContext(ctx, `INSERT INTO messages(
			conversation_id, author_id, thread_root_id, reference_message_id, body, rendered_body, idempotency_key, created_at)
			SELECT member.conversation_id, ?, ?, ?, ?, ?, ?, ? FROM conversation_members member
			WHERE member.conversation_id = ? AND member.user_id = ? AND (? IS NULL OR EXISTS(
				SELECT 1 FROM messages root WHERE root.id = ? AND root.conversation_id = member.conversation_id
				AND root.reply_to_id IS NULL AND root.thread_root_id IS NULL))
			AND (? IS NULL OR EXISTS(SELECT 1 FROM messages target
				WHERE target.id = ? AND target.conversation_id = member.conversation_id AND target.reply_to_id IS NULL
				AND ((? IS NULL AND target.thread_root_id IS NULL) OR
					(? IS NOT NULL AND (target.id = ? OR target.thread_root_id = ?)))))`,
			record.AuthorID, nullableInt64(record.ThreadRootID), nullableInt64(record.ReplyToMessageID),
			record.Body, record.RenderedBody, record.IdempotencyKey, unix(record.CreatedAt),
			record.ConversationID, record.AuthorID, nullableInt64(record.ThreadRootID), nullableInt64(record.ThreadRootID),
			nullableInt64(record.ReplyToMessageID), nullableInt64(record.ReplyToMessageID),
			nullableInt64(record.ThreadRootID), nullableInt64(record.ThreadRootID),
			nullableInt64(record.ThreadRootID), nullableInt64(record.ThreadRootID))
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
			ThreadRootID: record.ThreadRootID, ReplyToMessageID: record.ReplyToMessageID,
			Body: record.Body, RenderedBody: record.RenderedBody, CreatedAt: record.CreatedAt.UTC(),
		}
		event, err := insertMessageEvent(ctx, tx, "message.sent", item, record.CreatedAt)
		if err != nil {
			return err
		}
		if err := queueMentionedAgentTasks(ctx, tx, item, record.Mentions); err != nil {
			return err
		}
		result = message.Result{Message: item, Event: event}
		return recordMessageMutation(ctx, tx, record.AuthorID, record.IdempotencyKey, "send",
			fingerprint, result, record.CreatedAt)
	})
	return result, err
}

func queueMentionedAgentTasks(ctx context.Context, tx *sql.Tx, item message.Message, mentions []string) error {
	for _, username := range mentions {
		_, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO agent_tasks(
			agent_id, conversation_id, owner_id, invoking_message_id, thread_root_id, state, created_at)
			SELECT agent.user_id, ?, ?, ?, ?, 'queued', ?
			FROM users identity JOIN agents agent ON agent.user_id = identity.id
			JOIN agent_conversation_grants grant_row ON grant_row.agent_id = agent.user_id
				AND grant_row.conversation_id = ?
			JOIN conversations c ON c.id = grant_row.conversation_id
			WHERE identity.username = ? COLLATE NOCASE AND identity.principal_kind = 'agent'
				AND agent.revoked_at IS NULL AND c.agent_policy = 'explicit'`,
			item.ConversationID, item.AuthorID, item.ID, nullableInt64(item.ThreadRootID),
			unix(item.CreatedAt), item.ConversationID, username)
		if err != nil {
			return err
		}
	}
	return nil
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
			AuthorID         int64     `json:"author_id"`
			ThreadRootID     *int64    `json:"thread_root_id,omitempty"`
			ReplyToMessageID *int64    `json:"reply_to_message_id,omitempty"`
			Body             string    `json:"body"`
			RenderedBody     string    `json:"rendered_body"`
			CreatedAt        time.Time `json:"created_at"`
		}{item.AuthorID, item.ThreadRootID, item.ReplyToMessageID, item.Body, item.RenderedBody, item.CreatedAt})
	case "message.edited":
		return json.Marshal(struct {
			Body         string               `json:"body"`
			RenderedBody string               `json:"rendered_body"`
			EditedAt     time.Time            `json:"edited_at"`
			ThreadRootID *int64               `json:"thread_root_id,omitempty"`
			InlineApps   []message.InlineApp  `json:"inline_apps,omitempty"`
			Questions    []agenttask.Question `json:"questions,omitempty"`
		}{item.Body, item.RenderedBody, *item.EditedAt, item.ThreadRootID, item.InlineApps, item.Questions})
	case "message.deleted":
		return json.Marshal(struct {
			DeletedAt    time.Time `json:"deleted_at"`
			ThreadRootID *int64    `json:"thread_root_id,omitempty"`
		}{*item.DeletedAt, item.ThreadRootID})
	default:
		return nil, errors.New("unknown message event type")
	}
}

func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
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
