package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

func TestAuthStoreBootstrapRefusesSecondAdministrator(t *testing.T) {
	store, _ := newTestAuthStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	first := auth.BootstrapRecord{Username: "admin", PasswordHash: "hash-one", Admin: true, CreatedAt: now}
	if err := store.Bootstrap(context.Background(), first); err != nil {
		t.Fatalf("first Bootstrap: %v", err)
	}
	second := auth.BootstrapRecord{Username: "second", PasswordHash: "hash-two", Admin: true, CreatedAt: now}
	if err := store.Bootstrap(context.Background(), second); !errors.Is(err, auth.ErrAlreadyBootstrapped) {
		t.Fatalf("second Bootstrap error = %v, want ErrAlreadyBootstrapped", err)
	}
}

func TestAuthStoreInviteIsSingleUseAndExpires(t *testing.T) {
	store, db := newTestAuthStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	bootstrapTestAdmin(t, store, now)

	validHash := sha256.Sum256([]byte("valid invite"))
	if err := store.CreateInvite(context.Background(), auth.InviteRecord{
		TokenHash: validHash, CreatedBy: 1, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	redeem := auth.RedeemRecord{
		Username: "member", PasswordHash: "member-hash", InviteHash: validHash,
		SessionHash: sha256.Sum256([]byte("first session")), Now: now, SessionExpiresAt: now.Add(time.Hour),
	}
	member, err := store.RedeemInvite(context.Background(), redeem)
	if err != nil {
		t.Fatalf("RedeemInvite: %v", err)
	}
	if member.Username != "member" || member.Admin {
		t.Fatalf("redeemed user = %#v", member)
	}
	if _, err := store.RedeemInvite(context.Background(), auth.RedeemRecord{
		Username: "other", PasswordHash: "other-hash", InviteHash: validHash,
		SessionHash: sha256.Sum256([]byte("other session")), Now: now, SessionExpiresAt: now.Add(time.Hour),
	}); !errors.Is(err, auth.ErrInvalidInvite) {
		t.Fatalf("second redemption error = %v, want ErrInvalidInvite", err)
	}

	expiredHash := sha256.Sum256([]byte("expired invite"))
	if err := store.CreateInvite(context.Background(), auth.InviteRecord{
		TokenHash: expiredHash, CreatedBy: 1, CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("create expired invite: %v", err)
	}
	if _, err := store.RedeemInvite(context.Background(), auth.RedeemRecord{
		Username: "late", PasswordHash: "late-hash", InviteHash: expiredHash,
		SessionHash: sha256.Sum256([]byte("late session")), Now: now, SessionExpiresAt: now.Add(time.Hour),
	}); !errors.Is(err, auth.ErrInvalidInvite) {
		t.Fatalf("expired redemption error = %v, want ErrInvalidInvite", err)
	}

	var stored []byte
	if err := db.QueryRow("SELECT token_hash FROM invites WHERE id = 1").Scan(&stored); err != nil {
		t.Fatalf("read stored invite hash: %v", err)
	}
	if string(stored) != string(validHash[:]) {
		t.Fatal("database did not store only the invite SHA-256 hash")
	}
}

func TestAuthStoreReplaceSessionsIsAtomicAndStoresOnlyHashes(t *testing.T) {
	store, db := newTestAuthStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	bootstrapTestAdmin(t, store, now)
	first := sha256.Sum256([]byte("session one"))
	second := sha256.Sum256([]byte("session two"))
	for _, tokenHash := range [][32]byte{first, second} {
		if err := store.ReplaceUserSessions(context.Background(), auth.SessionRecord{
			UserID: 1, TokenHash: tokenHash, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatalf("ReplaceUserSessions: %v", err)
		}
	}

	if _, err := store.SessionUser(context.Background(), first, now); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("old session lookup error = %v, want ErrUnauthenticated", err)
	}
	user, err := store.SessionUser(context.Background(), second, now)
	if err != nil || user.ID != 1 {
		t.Fatalf("new session lookup = (%#v, %v)", user, err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sessions WHERE user_id = 1").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("session count = %d, want 1", count)
	}
	if _, err := store.SessionUser(context.Background(), second, now.Add(2*time.Hour)); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("expired session lookup error = %v, want ErrUnauthenticated", err)
	}
	if err := store.RevokeSession(context.Background(), second); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, err := store.SessionUser(context.Background(), second, now); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("revoked session lookup error = %v, want ErrUnauthenticated", err)
	}
}

func TestAuthStoreFailedReplacementPreservesPriorSession(t *testing.T) {
	store, _ := newTestAuthStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	bootstrapTestAdmin(t, store, now)
	if err := store.writer.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO users(username, password_hash, is_admin, created_at)
			VALUES ('member', 'member-hash', 0, ?)`, now.Unix())
		return err
	}); err != nil {
		t.Fatalf("create member fixture: %v", err)
	}
	adminToken := sha256.Sum256([]byte("admin session"))
	memberToken := sha256.Sum256([]byte("member session"))
	for userID, tokenHash := range map[int64][32]byte{1: adminToken, 2: memberToken} {
		if err := store.ReplaceUserSessions(context.Background(), auth.SessionRecord{
			UserID: userID, TokenHash: tokenHash, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatalf("seed user %d session: %v", userID, err)
		}
	}
	if err := store.ReplaceUserSessions(context.Background(), auth.SessionRecord{
		UserID: 1, TokenHash: memberToken, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err == nil {
		t.Fatal("colliding replacement error = nil, want unique-token failure")
	}
	user, err := store.SessionUser(context.Background(), adminToken, now)
	if err != nil || user.ID != 1 {
		t.Fatalf("prior session after failed replacement = (%#v, %v)", user, err)
	}
}

func TestAuthStoreCreateInviteRechecksAdministratorInWrite(t *testing.T) {
	store, _ := newTestAuthStore(t)
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	bootstrapTestAdmin(t, store, now)
	if err := store.writer.Do(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO users(username, password_hash, is_admin, created_at)
			VALUES ('member', 'member-hash', 0, ?)`, now.Unix())
		return err
	}); err != nil {
		t.Fatalf("create member fixture: %v", err)
	}

	err := store.CreateInvite(context.Background(), auth.InviteRecord{
		TokenHash: sha256.Sum256([]byte("member invite")), CreatedBy: 2,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("member CreateInvite error = %v, want ErrForbidden", err)
	}
}

func TestAuthStoreMapsSaturatedWriterToDomainBusy(t *testing.T) {
	db := openTestDB(t)
	writer, err := NewWriter(db, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	active := make(chan error, 1)
	go func() {
		active <- writer.Do(context.Background(), func(*sql.Tx) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	queued := make(chan error, 1)
	go func() { queued <- writer.Do(context.Background(), func(*sql.Tx) error { return nil }) }()
	waitForQueuedRequests(t, writer, 1)

	store := NewAuthStore(db, writer)
	err = store.Bootstrap(context.Background(), auth.BootstrapRecord{})
	if !errors.Is(err, auth.ErrBusy) {
		t.Fatalf("saturated Bootstrap error = %v, want auth.ErrBusy", err)
	}
	close(release)
	if err := <-active; err != nil {
		t.Fatalf("active write: %v", err)
	}
	if err := <-queued; err != nil {
		t.Fatalf("queued write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
}

func newTestAuthStore(t *testing.T) (*AuthStore, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	writer, err := NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})
	return NewAuthStore(db, writer), db
}

func bootstrapTestAdmin(t *testing.T, store *AuthStore, now time.Time) {
	t.Helper()
	if err := store.Bootstrap(context.Background(), auth.BootstrapRecord{
		Username: "admin", PasswordHash: "admin-hash", Admin: true, CreatedAt: now,
	}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
}
