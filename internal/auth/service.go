package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	inviteLifetime   = 24 * time.Hour
	sessionLifetime  = 30 * 24 * time.Hour
	defaultUserLimit = 20
	maxUserLimit     = 50
	maxUsernameBytes = 64
	minPasswordBytes = 12
	maxPasswordBytes = 128
)

const dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

var tokenEncoding = base64.RawURLEncoding.Strict()

// Service applies validation and cryptography before crossing the repository port.
type Service struct {
	repository Repository
	now        func() time.Time
	random     io.Reader
}

// NewService requires explicit production or deterministic test dependencies.
func NewService(repository Repository, now func() time.Time, random io.Reader) (*Service, error) {
	if repository == nil || now == nil || random == nil {
		return nil, fmt.Errorf("auth repository, clock, and randomness are required")
	}
	return &Service{repository: repository, now: now, random: random}, nil
}

func (s *Service) Bootstrap(ctx context.Context, command Bootstrap) error {
	if validateUsername(command.Username) != nil || validatePassword(command.Password) != nil {
		return ErrInvalidInput
	}
	hash, err := HashPassword(command.Password, s.random)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	return s.repository.Bootstrap(ctx, BootstrapRecord{
		Username: command.Username, PasswordHash: hash, Admin: true, CreatedAt: s.now().UTC(),
	})
}

func (s *Service) CreateInvite(ctx context.Context, creatorID int64) (Invite, error) {
	if creatorID <= 0 {
		return Invite{}, ErrForbidden
	}
	raw, encoded, err := s.newToken()
	if err != nil {
		return Invite{}, err
	}
	now := s.now().UTC()
	expires := now.Add(inviteLifetime)
	if err := s.repository.CreateInvite(ctx, InviteRecord{
		TokenHash: sha256.Sum256(raw[:]), CreatedBy: creatorID, CreatedAt: now, ExpiresAt: expires,
	}); err != nil {
		return Invite{}, err
	}
	return Invite{Token: encoded, ExpiresAt: expires}, nil
}

func (s *Service) Login(ctx context.Context, command Login) (Session, error) {
	if validateUsername(command.Username) != nil || validatePassword(command.Password) != nil {
		return Session{}, ErrInvalidCredentials
	}
	credential, err := s.repository.Credential(ctx, command.Username)
	hash := credential.PasswordHash
	missing := errors.Is(err, ErrCredentialNotFound)
	if missing {
		hash = dummyPasswordHash
	} else if err != nil {
		return Session{}, err
	}
	if !VerifyPassword(command.Password, hash) || missing {
		return Session{}, ErrInvalidCredentials
	}
	return s.replaceSessions(ctx, credential.User)
}

func (s *Service) RedeemInvite(ctx context.Context, command CreateUser) (Session, error) {
	if validateUsername(command.Username) != nil || validatePassword(command.Password) != nil {
		return Session{}, ErrInvalidInput
	}
	inviteToken, err := DecodeToken(command.InviteToken)
	if err != nil {
		return Session{}, ErrInvalidInvite
	}
	passwordHash, err := HashPassword(command.Password, s.random)
	if err != nil {
		return Session{}, fmt.Errorf("hash member password: %w", err)
	}
	sessionToken, encoded, err := s.newToken()
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	expires := now.Add(sessionLifetime)
	user, err := s.repository.RedeemInvite(ctx, RedeemRecord{
		Username: command.Username, PasswordHash: passwordHash,
		InviteHash: sha256.Sum256(inviteToken[:]), SessionHash: sha256.Sum256(sessionToken[:]),
		Now: now, SessionExpiresAt: expires,
	})
	if err != nil {
		return Session{}, err
	}
	return Session{Token: encoded, ExpiresAt: expires, User: user}, nil
}

func (s *Service) Authenticate(ctx context.Context, raw [32]byte) (User, error) {
	return s.repository.SessionUser(ctx, sha256.Sum256(raw[:]), s.now().UTC())
}

func (s *Service) Revoke(ctx context.Context, raw [32]byte) error {
	return s.repository.RevokeSession(ctx, sha256.Sum256(raw[:]))
}

func (s *Service) SetAvatar(ctx context.Context, userID int64, mime string, data []byte) error {
	if userID <= 0 || len(data) == 0 || len(data) > MaxAvatarBytes || (mime != "image/png" && mime != "image/jpeg" && mime != "image/webp") {
		return ErrInvalidInput
	}
	owned := append([]byte(nil), data...)
	return s.repository.SetAvatar(ctx, userID, Avatar{MIME: mime, Data: owned, UpdatedAt: s.now().UTC()})
}

func (s *Service) DeleteAvatar(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return ErrInvalidInput
	}
	return s.repository.DeleteAvatar(ctx, userID)
}

func (s *Service) Avatar(ctx context.Context, requesterID, userID int64) (Avatar, error) {
	if requesterID <= 0 || userID <= 0 {
		return Avatar{}, ErrInvalidInput
	}
	return s.repository.Avatar(ctx, requesterID, userID)
}

// FindUsers returns a bounded public identity projection for an authenticated human.
func (s *Service) FindUsers(ctx context.Context, command FindUsers) (UserDirectory, error) {
	if command.RequesterID <= 0 || len(command.Query) > maxUsernameBytes || !utf8.ValidString(command.Query) ||
		strings.TrimSpace(command.Query) != command.Query || command.Limit < 0 || command.Limit > maxUserLimit {
		return UserDirectory{}, ErrInvalidInput
	}
	if command.Limit == 0 {
		command.Limit = defaultUserLimit
	}
	users, err := s.repository.SearchUsers(ctx, command.RequesterID, command.Query, command.Limit)
	if err != nil {
		return UserDirectory{}, err
	}
	return UserDirectory{Users: users}, nil
}

func (s *Service) NewCSRFToken() (string, error) {
	_, encoded, err := s.newToken()
	return encoded, err
}

func (s *Service) replaceSessions(ctx context.Context, user User) (Session, error) {
	raw, encoded, err := s.newToken()
	if err != nil {
		return Session{}, err
	}
	now := s.now().UTC()
	expires := now.Add(sessionLifetime)
	if err := s.repository.ReplaceUserSessions(ctx, SessionRecord{
		UserID: user.ID, TokenHash: sha256.Sum256(raw[:]), CreatedAt: now, ExpiresAt: expires,
	}); err != nil {
		return Session{}, err
	}
	return Session{Token: encoded, ExpiresAt: expires, User: user}, nil
}

func (s *Service) newToken() ([tokenBytes]byte, string, error) {
	var raw [tokenBytes]byte
	if _, err := io.ReadFull(s.random, raw[:]); err != nil {
		return raw, "", fmt.Errorf("read authentication randomness: %w", err)
	}
	return raw, tokenEncoding.EncodeToString(raw[:]), nil
}

// DecodeToken accepts only the canonical 32-byte base64url representation.
func DecodeToken(encoded string) ([tokenBytes]byte, error) {
	var raw [tokenBytes]byte
	if len(encoded) != tokenEncoding.EncodedLen(tokenBytes) {
		return raw, ErrUnauthenticated
	}
	written, err := tokenEncoding.Decode(raw[:], []byte(encoded))
	if err != nil || written != tokenBytes {
		return raw, ErrUnauthenticated
	}
	return raw, nil
}

func validateUsername(username string) error {
	if username == "" || len(username) > maxUsernameBytes || !utf8.ValidString(username) || strings.TrimSpace(username) != username {
		return ErrInvalidInput
	}
	for index, character := range username {
		if unicode.IsLetter(character) || unicode.IsDigit(character) || (index > 0 && (character == '.' || character == '_' || character == '-')) {
			continue
		}
		return ErrInvalidInput
	}
	return nil
}

func validatePassword(password string) error {
	if len(password) < minPasswordBytes || len(password) > maxPasswordBytes || !utf8.ValidString(password) {
		return ErrInvalidInput
	}
	return nil
}
