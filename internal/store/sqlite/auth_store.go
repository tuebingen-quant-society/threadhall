package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

// AuthStore implements auth.Repository over the shipped core schema.
type AuthStore struct {
	db     *sql.DB
	writer *Writer
}

func NewAuthStore(db *sql.DB, writer *Writer) *AuthStore {
	return &AuthStore{db: db, writer: writer}
}

func (s *AuthStore) Bootstrap(ctx context.Context, record auth.BootstrapRecord) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var users int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil {
			return err
		}
		if users != 0 {
			return auth.ErrAlreadyBootstrapped
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO users(username, password_hash, is_admin, created_at)
			VALUES (?, ?, ?, ?)`, record.Username, record.PasswordHash, record.Admin, unix(record.CreatedAt))
		return err
	})
}

func (s *AuthStore) CreateInvite(ctx context.Context, record auth.InviteRecord) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO invites(token_hash, created_by, expires_at, created_at)
			SELECT ?, id, ?, ? FROM users WHERE id = ? AND is_admin = 1`,
			record.TokenHash[:], unix(record.ExpiresAt), unix(record.CreatedAt), record.CreatedBy)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return auth.ErrForbidden
		}
		return nil
	})
}

func (s *AuthStore) Credential(ctx context.Context, username string) (auth.Credential, error) {
	var credential auth.Credential
	var admin bool
	var created int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, created_at
		FROM users WHERE username = ?`, username).Scan(
		&credential.User.ID, &credential.User.Username, &credential.PasswordHash, &admin, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Credential{}, auth.ErrCredentialNotFound
	}
	if err != nil {
		return auth.Credential{}, err
	}
	credential.User.Admin = admin
	credential.User.CreatedAt = time.Unix(created, 0).UTC()
	return credential, nil
}

func (s *AuthStore) ReplaceUserSessions(ctx context.Context, record auth.SessionRecord) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", record.UserID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO sessions(user_id, token_hash, expires_at, created_at)
			VALUES (?, ?, ?, ?)`, record.UserID, record.TokenHash[:], unix(record.ExpiresAt), unix(record.CreatedAt))
		return err
	})
}

func (s *AuthStore) RedeemInvite(ctx context.Context, record auth.RedeemRecord) (auth.User, error) {
	var user auth.User
	err := s.write(ctx, func(tx *sql.Tx) error {
		var inviteID int64
		err := tx.QueryRowContext(ctx, `
			SELECT id FROM invites
			WHERE token_hash = ? AND redeemed_by IS NULL AND expires_at > ?`,
			record.InviteHash[:], unix(record.Now)).Scan(&inviteID)
		if errors.Is(err, sql.ErrNoRows) {
			return auth.ErrInvalidInvite
		}
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username = ?)", record.Username).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return auth.ErrUsernameUnavailable
		}
		result, err := tx.ExecContext(ctx, `
			INSERT INTO users(username, password_hash, is_admin, created_at)
			VALUES (?, ?, 0, ?)`, record.Username, record.PasswordHash, unix(record.Now))
		if err != nil {
			return err
		}
		user.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		result, err = tx.ExecContext(ctx, `
			UPDATE invites SET redeemed_by = ?, redeemed_at = ?
			WHERE id = ? AND redeemed_by IS NULL AND expires_at > ?`,
			user.ID, unix(record.Now), inviteID, unix(record.Now))
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			return auth.ErrInvalidInvite
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO sessions(user_id, token_hash, expires_at, created_at)
			VALUES (?, ?, ?, ?)`, user.ID, record.SessionHash[:], unix(record.SessionExpiresAt), unix(record.Now))
		return err
	})
	if err != nil {
		return auth.User{}, err
	}
	user.Username = record.Username
	user.CreatedAt = record.Now.UTC()
	return user, nil
}

func (s *AuthStore) SessionUser(ctx context.Context, tokenHash [32]byte, now time.Time) (auth.User, error) {
	var user auth.User
	var created int64
	err := s.db.QueryRowContext(ctx, `
		SELECT users.id, users.username, users.is_admin, users.created_at
		FROM sessions JOIN users ON users.id = sessions.user_id
		WHERE sessions.token_hash = ? AND sessions.expires_at > ?`, tokenHash[:], unix(now)).Scan(
		&user.ID, &user.Username, &user.Admin, &created,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, auth.ErrUnauthenticated
	}
	if err != nil {
		return auth.User{}, err
	}
	user.CreatedAt = time.Unix(created, 0).UTC()
	return user, nil
}

func (s *AuthStore) RevokeSession(ctx context.Context, tokenHash [32]byte) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE token_hash = ?", tokenHash[:])
		return err
	})
}

func (s *AuthStore) write(ctx context.Context, fn WriteFunc) error {
	err := s.writer.Do(ctx, fn)
	if errors.Is(err, ErrBusy) {
		return auth.ErrBusy
	}
	return err
}

func unix(value time.Time) int64 { return value.UTC().Unix() }
