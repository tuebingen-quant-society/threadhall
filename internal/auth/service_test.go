package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestServiceBootstrapValidatesAndHashesPassword(t *testing.T) {
	repository := &recordingRepository{}
	service := newTestService(t, repository, bytes.Repeat([]byte{0x11}, 32))

	err := service.Bootstrap(context.Background(), Bootstrap{
		Username: "admin",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if repository.bootstrap.Username != "admin" || !repository.bootstrap.Admin {
		t.Fatalf("bootstrap record = %#v", repository.bootstrap)
	}
	if repository.bootstrap.PasswordHash == "correct horse battery staple" {
		t.Fatal("Bootstrap stored the plaintext password")
	}
	if !VerifyPassword("correct horse battery staple", repository.bootstrap.PasswordHash) {
		t.Fatal("Bootstrap stored an unverifiable password hash")
	}

	repository.bootstrapErr = ErrAlreadyBootstrapped
	if err := service.Bootstrap(context.Background(), Bootstrap{
		Username: "other", Password: "another correct password",
	}); !errors.Is(err, ErrAlreadyBootstrapped) {
		t.Fatalf("second Bootstrap error = %v, want ErrAlreadyBootstrapped", err)
	}
}

func TestServiceCreatesHashedExpiringInvite(t *testing.T) {
	random := bytes.Repeat([]byte{0x23}, tokenBytes)
	repository := &recordingRepository{}
	service := newTestService(t, repository, random)

	invite, err := service.CreateInvite(context.Background(), 7)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	raw, err := DecodeToken(invite.Token)
	if err != nil {
		t.Fatalf("decode returned invite: %v", err)
	}
	wantHash := sha256.Sum256(raw[:])
	if repository.invite.TokenHash != wantHash {
		t.Fatal("repository did not receive SHA-256 of the invite token")
	}
	if repository.invite.CreatedBy != 7 || !repository.invite.ExpiresAt.Equal(testNow.Add(24*time.Hour)) {
		t.Fatalf("invite record = %#v", repository.invite)
	}
	if !invite.ExpiresAt.Equal(repository.invite.ExpiresAt) {
		t.Fatalf("returned expiry = %v, want %v", invite.ExpiresAt, repository.invite.ExpiresAt)
	}
}

func TestServiceLoginUsesGenericFailureAndRotatesHashedToken(t *testing.T) {
	passwordHash, err := HashPassword("correct horse battery staple", bytes.NewReader(bytes.Repeat([]byte{9}, 16)))
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	repository := &recordingRepository{credential: Credential{User: User{ID: 5, Username: "member"}, PasswordHash: passwordHash}}
	service := newTestService(t, repository, bytes.Repeat([]byte{0x35}, tokenBytes))

	session, err := service.Login(context.Background(), Login{Username: "member", Password: "correct horse battery staple"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	raw, err := DecodeToken(session.Token)
	if err != nil {
		t.Fatalf("decode returned session: %v", err)
	}
	if repository.replacement.UserID != 5 || repository.replacement.TokenHash != sha256.Sum256(raw[:]) {
		t.Fatalf("session replacement = %#v", repository.replacement)
	}
	if !repository.replacement.ExpiresAt.Equal(testNow.Add(30 * 24 * time.Hour)) {
		t.Fatalf("session expiry = %v", repository.replacement.ExpiresAt)
	}

	for _, login := range []Login{
		{Username: "member", Password: "wrong password here"},
		{Username: "missing", Password: "wrong password here"},
	} {
		repository.credentialErr = nil
		if login.Username == "missing" {
			repository.credentialErr = ErrCredentialNotFound
		}
		if _, err := service.Login(context.Background(), login); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login(%q) error = %v, want generic ErrInvalidCredentials", login.Username, err)
		}
	}
	repository.credentialErr = fmt.Errorf("credential lookup: %w", ErrCredentialNotFound)
	if _, err := service.Login(context.Background(), Login{
		Username: "missing", Password: "wrong password here",
	}); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login with wrapped missing-user error = %v, want generic ErrInvalidCredentials", err)
	}
}

func TestServiceRejectsBoundedAuthenticationInputsBeforePersistence(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{name: "empty username", password: "correct horse battery staple"},
		{name: "username over byte limit", username: string(bytes.Repeat([]byte{'a'}, 65)), password: "correct horse battery staple"},
		{name: "username contains space", username: "invalid name", password: "correct horse battery staple"},
		{name: "password under minimum", username: "member", password: "short"},
		{name: "password over byte limit", username: "member", password: string(bytes.Repeat([]byte{'p'}, 129))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &recordingRepository{}
			service := newTestService(t, repository, bytes.Repeat([]byte{1}, 64))
			if err := service.Bootstrap(context.Background(), Bootstrap{
				Username: test.username, Password: test.password,
			}); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Bootstrap error = %v, want ErrInvalidInput", err)
			}
			if repository.bootstrap.Username != "" {
				t.Fatalf("invalid command reached repository: %#v", repository.bootstrap)
			}
		})
	}
}

func TestServiceRedeemsRawInviteIntoHashedSession(t *testing.T) {
	inviteRaw := [tokenBytes]byte{}
	for index := range inviteRaw {
		inviteRaw[index] = 0x29
	}
	random := append(bytes.Repeat([]byte{0x18}, passwordSaltBytes), bytes.Repeat([]byte{0x39}, tokenBytes)...)
	repository := &recordingRepository{redeemUser: User{ID: 9, Username: "new-member"}}
	service := newTestService(t, repository, random)
	session, err := service.RedeemInvite(context.Background(), CreateUser{
		Username: "new-member", Password: "correct horse battery staple",
		InviteToken: tokenEncoding.EncodeToString(inviteRaw[:]),
	})
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if !VerifyPassword("correct horse battery staple", repository.redeem.PasswordHash) {
		t.Fatal("redeemed password hash does not verify")
	}
	if repository.redeem.InviteHash != sha256.Sum256(inviteRaw[:]) {
		t.Fatal("repository did not receive the invite SHA-256 hash")
	}
	sessionRaw, err := DecodeToken(session.Token)
	if err != nil {
		t.Fatalf("decode returned session: %v", err)
	}
	if repository.redeem.SessionHash != sha256.Sum256(sessionRaw[:]) || session.User.ID != 9 {
		t.Fatalf("redeem record/session = (%#v, %#v)", repository.redeem, session)
	}
}

func TestServiceHashesTokensForLookupAndRevocation(t *testing.T) {
	repository := &recordingRepository{sessionUser: User{ID: 3, Username: "member"}}
	service := newTestService(t, repository, bytes.Repeat([]byte{0x45}, tokenBytes))
	var raw [tokenBytes]byte
	for index := range raw {
		raw[index] = 0x28
	}
	user, err := service.Authenticate(context.Background(), raw)
	if err != nil || user.ID != 3 {
		t.Fatalf("Authenticate = (%#v, %v)", user, err)
	}
	wantHash := sha256.Sum256(raw[:])
	if repository.lookupHash != wantHash || !repository.lookupNow.Equal(testNow) {
		t.Fatalf("lookup hash/time = (%x, %v)", repository.lookupHash, repository.lookupNow)
	}
	if err := service.Revoke(context.Background(), raw); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if repository.revokeHash != wantHash {
		t.Fatal("revoke did not receive the session SHA-256 hash")
	}
	csrf, err := service.NewCSRFToken()
	if err != nil {
		t.Fatalf("NewCSRFToken: %v", err)
	}
	if decoded, err := DecodeToken(csrf); err != nil || len(decoded) != tokenBytes {
		t.Fatalf("CSRF token decode = (%x, %v)", decoded, err)
	}
}

var testNow = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

func newTestService(t *testing.T, repository Repository, random []byte) *Service {
	t.Helper()
	service, err := NewService(repository, func() time.Time { return testNow }, bytes.NewReader(random))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type recordingRepository struct {
	bootstrap     BootstrapRecord
	bootstrapErr  error
	invite        InviteRecord
	credential    Credential
	credentialErr error
	replacement   SessionRecord
	redeem        RedeemRecord
	redeemUser    User
	lookupHash    [32]byte
	lookupNow     time.Time
	sessionUser   User
	revokeHash    [32]byte
}

func (r *recordingRepository) Bootstrap(_ context.Context, record BootstrapRecord) error {
	r.bootstrap = record
	return r.bootstrapErr
}

func (r *recordingRepository) CreateInvite(_ context.Context, record InviteRecord) error {
	r.invite = record
	return nil
}

func (r *recordingRepository) Credential(_ context.Context, _ string) (Credential, error) {
	return r.credential, r.credentialErr
}

func (r *recordingRepository) ReplaceUserSessions(_ context.Context, record SessionRecord) error {
	r.replacement = record
	return nil
}

func (r *recordingRepository) RedeemInvite(_ context.Context, record RedeemRecord) (User, error) {
	r.redeem = record
	return r.redeemUser, nil
}

func (r *recordingRepository) SessionUser(_ context.Context, hash [32]byte, now time.Time) (User, error) {
	r.lookupHash, r.lookupNow = hash, now
	return r.sessionUser, nil
}

func (r *recordingRepository) RevokeSession(_ context.Context, hash [32]byte) error {
	r.revokeHash = hash
	return nil
}
