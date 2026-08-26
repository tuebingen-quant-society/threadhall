package auth

import (
	"context"
	"errors"
	"time"
)

var (
	ErrAlreadyBootstrapped = errors.New("administrator already bootstrapped")
	ErrCredentialNotFound  = errors.New("credential not found")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrInvalidInput        = errors.New("invalid authentication input")
	ErrInvalidInvite       = errors.New("invalid invite")
	ErrUsernameUnavailable = errors.New("username unavailable")
	ErrUnauthenticated     = errors.New("authentication required")
	ErrForbidden           = errors.New("administrator required")
	ErrBusy                = errors.New("authentication persistence is busy")
)

type BootstrapRecord struct {
	Username, PasswordHash string
	Admin                  bool
	CreatedAt              time.Time
}

type Credential struct {
	User         User
	PasswordHash string
}

type InviteRecord struct {
	TokenHash            [32]byte
	CreatedBy            int64
	CreatedAt, ExpiresAt time.Time
}

type RedeemRecord struct {
	Username, PasswordHash  string
	InviteHash, SessionHash [32]byte
	Now, SessionExpiresAt   time.Time
}

type SessionRecord struct {
	UserID               int64
	TokenHash            [32]byte
	CreatedAt, ExpiresAt time.Time
}

// Repository is the SQLite-independent persistence port. Raw tokens and
// passwords never cross it.
type Repository interface {
	Bootstrap(context.Context, BootstrapRecord) error
	CreateInvite(context.Context, InviteRecord) error
	Credential(context.Context, string) (Credential, error)
	ReplaceUserSessions(context.Context, SessionRecord) error
	RedeemInvite(context.Context, RedeemRecord) (User, error)
	SearchUsers(context.Context, int64, string, int) ([]DirectoryUser, error)
	SessionUser(context.Context, [32]byte, time.Time) (User, error)
	RevokeSession(context.Context, [32]byte) error
}
