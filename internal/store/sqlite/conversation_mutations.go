package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/mattn/go-sqlite3"
	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func (s *ConversationStore) CreateChannel(ctx context.Context, record conversation.ChannelRecord) (conversation.Conversation, error) {
	fingerprint := mustFingerprint(record.Kind, record.Name, record.MemberIDs)
	var created conversation.Conversation
	err := s.write(ctx, func(tx *sql.Tx) error {
		for _, memberID := range record.MemberIDs {
			var human bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND principal_kind = 'human')`, memberID).Scan(&human); err != nil {
				return err
			}
			if !human {
				return conversation.ErrNotFound
			}
		}
		id, found, err := findMutation(ctx, tx, record.CreatorID, record.IdempotencyKey, "create_channel", fingerprint)
		if err != nil {
			return err
		}
		if found {
			created, err = conversationByID(ctx, tx, id)
			return err
		}
		legacy, legacyErr := legacyConversationByKey(ctx, tx, record.CreatorID, record.IdempotencyKey)
		if legacyErr == nil {
			if legacy.Kind != record.Kind || legacy.Name != record.Name {
				return conversation.ErrConflict
			}
			if err := recordMutation(ctx, tx, record.CreatorID, record.IdempotencyKey, "create_channel",
				fingerprint, legacy.ID, unix(legacy.CreatedAt)); err != nil {
				return err
			}
			created = legacy
			return nil
		}
		if !errors.Is(legacyErr, sql.ErrNoRows) {
			return legacyErr
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO conversations(kind, name, created_by, idempotency_key, created_at)
			SELECT ?, ?, id, ?, ? FROM users WHERE id = ? AND principal_kind = 'human'`,
			record.Kind, record.Name, record.IdempotencyKey, unix(record.CreatedAt), record.CreatorID)
		if err != nil {
			return mapConversationConstraint(err)
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			if err != nil {
				return err
			}
			return conversation.ErrNotFound
		}
		created.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		created.Kind, created.Name = record.Kind, record.Name
		created.CreatedBy, created.CreatedAt = record.CreatorID, record.CreatedAt.UTC()
		if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (?, ?, ?)`, created.ID, record.CreatorID, unix(record.CreatedAt)); err != nil {
			return err
		}
		for _, memberID := range record.MemberIDs {
			if memberID == record.CreatorID {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (?, ?, ?)`, created.ID, memberID, unix(record.CreatedAt)); err != nil {
				return err
			}
		}
		if err := recordMutation(ctx, tx, record.CreatorID, record.IdempotencyKey, "create_channel", fingerprint, created.ID, unix(record.CreatedAt)); err != nil {
			return err
		}
		return insertConversationEvent(ctx, tx, created.ID, record.CreatorID, "conversation.created", fingerprint, unix(record.CreatedAt))
	})
	return created, err
}

func (s *ConversationStore) CreateDM(ctx context.Context, record conversation.DMRecord) (conversation.Conversation, error) {
	fingerprint := mustFingerprint(record.UserLowID, record.UserHighID)
	var created conversation.Conversation
	err := s.write(ctx, func(tx *sql.Tx) error {
		id, found, err := findMutation(ctx, tx, record.RequesterID, record.IdempotencyKey, "create_dm", fingerprint)
		if err != nil {
			return err
		}
		if found {
			created, err = conversationByID(ctx, tx, id)
			return err
		}
		legacy, exact, legacyErr := legacyDMByKey(ctx, tx, record.RequesterID, record.IdempotencyKey,
			record.UserLowID, record.UserHighID)
		if legacyErr == nil {
			if !exact {
				return conversation.ErrConflict
			}
			if err := recordMutation(ctx, tx, record.RequesterID, record.IdempotencyKey, "create_dm",
				fingerprint, legacy.ID, unix(legacy.CreatedAt)); err != nil {
				return err
			}
			created = legacy
			return nil
		}
		if !errors.Is(legacyErr, sql.ErrNoRows) {
			return legacyErr
		}
		var users int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users
			WHERE id IN (?, ?) AND principal_kind = 'human'`, record.UserLowID, record.UserHighID).Scan(&users); err != nil {
			return err
		}
		if users != 2 {
			return conversation.ErrNotFound
		}
		created, err = conversationByDMPair(ctx, tx, record.UserLowID, record.UserHighID)
		inserted := false
		if errors.Is(err, sql.ErrNoRows) {
			created, err = insertDM(ctx, tx, record)
			inserted = err == nil
		}
		if err != nil {
			return err
		}
		if err := recordMutation(ctx, tx, record.RequesterID, record.IdempotencyKey, "create_dm", fingerprint, created.ID, unix(record.CreatedAt)); err != nil {
			return err
		}
		if inserted {
			return insertConversationEvent(ctx, tx, created.ID, record.RequesterID, "conversation.created", fingerprint, unix(record.CreatedAt))
		}
		return nil
	})
	return created, err
}

func insertDM(ctx context.Context, tx *sql.Tx, record conversation.DMRecord) (conversation.Conversation, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO conversations(kind, name, created_by, dm_user_low, dm_user_high, idempotency_key, created_at)
		VALUES ('dm', NULL, ?, ?, ?, ?, ?)`, record.RequesterID, record.UserLowID, record.UserHighID,
		record.IdempotencyKey, unix(record.CreatedAt))
	if err != nil {
		return conversation.Conversation{}, mapConversationConstraint(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return conversation.Conversation{}, err
	}
	result, err = tx.ExecContext(ctx, `
		INSERT INTO conversation_members(conversation_id, user_id, joined_at)
		SELECT ?, id, ? FROM users WHERE id IN (?, ?)`, id, unix(record.CreatedAt), record.UserLowID, record.UserHighID)
	if err != nil {
		return conversation.Conversation{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 2 {
		if err != nil {
			return conversation.Conversation{}, err
		}
		return conversation.Conversation{}, conversation.ErrNotFound
	}
	return conversation.Conversation{ID: id, Kind: conversation.KindDM, CreatedBy: record.RequesterID, CreatedAt: record.CreatedAt.UTC()}, nil
}

func (s *ConversationStore) AddMember(ctx context.Context, record conversation.MemberRecord) error {
	return s.changeMember(ctx, record, true)
}

func (s *ConversationStore) RemoveMember(ctx context.Context, record conversation.MemberRecord) error {
	return s.changeMember(ctx, record, false)
}

func (s *ConversationStore) changeMember(ctx context.Context, record conversation.MemberRecord, add bool) error {
	operation, eventKind := "remove_member", "conversation.member_removed"
	if add {
		operation, eventKind = "add_member", "conversation.member_added"
	}
	fingerprint := mustFingerprint(record.ConversationID, record.UserID)
	return s.write(ctx, func(tx *sql.Tx) error {
		var admin bool
		err := tx.QueryRowContext(ctx, `SELECT is_admin FROM users
			WHERE id = ? AND principal_kind = 'human'`, record.ActorID).Scan(&admin)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !admin) {
			return conversation.ErrForbidden
		}
		if err != nil {
			return err
		}
		var namedChannel bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversations
			WHERE id = ? AND kind IN ('channel', 'private'))`, record.ConversationID).Scan(&namedChannel); err != nil {
			return err
		}
		if !namedChannel {
			return conversation.ErrNotFound
		}
		_, found, err := findMutation(ctx, tx, record.ActorID, record.IdempotencyKey, operation, fingerprint)
		if err != nil || found {
			return err
		}
		if err := rejectLegacyIdempotency(ctx, tx, record.ActorID, record.IdempotencyKey); err != nil {
			return err
		}
		var humanExists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users
			WHERE id = ? AND principal_kind = 'human')`, record.UserID).Scan(&humanExists); err != nil {
			return err
		}
		if !humanExists {
			return conversation.ErrNotFound
		}
		var result sql.Result
		if add {
			result, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO conversation_members(conversation_id, user_id, joined_at)
				VALUES (?, ?, ?)`, record.ConversationID, record.UserID, unix(record.ChangedAt))
		} else {
			result, err = tx.ExecContext(ctx, `DELETE FROM conversation_members
				WHERE conversation_id = ? AND user_id = ?`, record.ConversationID, record.UserID)
		}
		if err != nil {
			return mapConversationConstraint(err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if err := recordMutation(ctx, tx, record.ActorID, record.IdempotencyKey, operation, fingerprint,
			record.ConversationID, unix(record.ChangedAt)); err != nil {
			return err
		}
		if changed == 0 {
			return nil
		}
		return insertConversationEvent(ctx, tx, record.ConversationID, record.ActorID, eventKind,
			fingerprint, unix(record.ChangedAt))
	})
}

func findMutation(ctx context.Context, tx *sql.Tx, actorID int64, key, operation, fingerprint string) (int64, bool, error) {
	var storedOperation, storedFingerprint string
	var conversationID int64
	err := tx.QueryRowContext(ctx, `SELECT operation, fingerprint, conversation_id
		FROM conversation_mutations WHERE actor_id = ? AND idempotency_key = ?`, actorID, key).
		Scan(&storedOperation, &storedFingerprint, &conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if storedOperation != operation || storedFingerprint != fingerprint {
		return 0, false, conversation.ErrConflict
	}
	return conversationID, true, nil
}

func recordMutation(ctx context.Context, tx *sql.Tx, actorID int64, key, operation, fingerprint string, conversationID, createdAt int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO conversation_mutations(
		actor_id, idempotency_key, operation, fingerprint, conversation_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, actorID, key, operation, fingerprint, conversationID, createdAt)
	return mapConversationConstraint(err)
}

func conversationByID(ctx context.Context, tx *sql.Tx, id int64) (conversation.Conversation, error) {
	return scanConversation(tx.QueryRowContext(ctx, `SELECT id, kind, name, created_by, created_at
		FROM conversations WHERE id = ?`, id))
}

func conversationByDMPair(ctx context.Context, tx *sql.Tx, low, high int64) (conversation.Conversation, error) {
	return scanConversation(tx.QueryRowContext(ctx, `SELECT id, kind, name, created_by, created_at
		FROM conversations WHERE dm_user_low = ? AND dm_user_high = ?`, low, high))
}

func insertConversationEvent(ctx context.Context, tx *sql.Tx, id, actorID int64, kind, payload string, createdAt int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO events(conversation_id, actor_id, kind, entity_id, payload, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, id, actorID, kind, id, payload, createdAt)
	return err
}

func mustFingerprint(values ...any) string {
	encoded, err := json.Marshal(values)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func mapConversationConstraint(err error) error {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrConstraint {
		return conversation.ErrConflict
	}
	return err
}

func (s *ConversationStore) write(ctx context.Context, fn WriteFunc) error {
	err := s.writer.Do(ctx, fn)
	if errors.Is(err, ErrBusy) {
		return conversation.ErrBusy
	}
	return err
}
