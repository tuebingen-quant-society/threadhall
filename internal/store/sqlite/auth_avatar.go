package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

func (s *AuthStore) SetAvatar(ctx context.Context, userID int64, avatar auth.Avatar) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE users SET avatar_mime = ?, avatar_data = ?, avatar_updated_at = ?
			WHERE id = ? AND principal_kind = 'human'`, avatar.MIME, avatar.Data, unix(avatar.UpdatedAt), userID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return auth.ErrCredentialNotFound
		}
		return nil
	})
}

func (s *AuthStore) DeleteAvatar(ctx context.Context, userID int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE users SET avatar_mime = NULL, avatar_data = NULL, avatar_updated_at = NULL
			WHERE id = ? AND principal_kind = 'human'`, userID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return auth.ErrCredentialNotFound
		}
		return nil
	})
}

func (s *AuthStore) Avatar(ctx context.Context, requesterID, userID int64) (auth.Avatar, error) {
	var avatar auth.Avatar
	var updatedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT target.avatar_mime, target.avatar_data, target.avatar_updated_at
		FROM users requester JOIN users target ON target.id = ? AND target.principal_kind = 'human'
		WHERE requester.id = ? AND requester.principal_kind = 'human' AND target.avatar_data IS NOT NULL`, userID, requesterID).
		Scan(&avatar.MIME, &avatar.Data, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var requester bool
		if scanErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ? AND principal_kind = 'human')`, requesterID).Scan(&requester); scanErr != nil {
			return auth.Avatar{}, scanErr
		}
		if !requester {
			return auth.Avatar{}, auth.ErrUnauthenticated
		}
		return auth.Avatar{}, auth.ErrCredentialNotFound
	}
	if err != nil {
		return auth.Avatar{}, err
	}
	avatar.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return avatar, nil
}
