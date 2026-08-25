package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func TestLoginGeneratesOutgoingCSRFBeforeReplacingPersistedSession(t *testing.T) {
	db, service := newPersistentAuthService(t)
	if err := service.Bootstrap(context.Background(), auth.Bootstrap{
		Username: "admin", Password: "correct horse battery staple",
	}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if _, err := service.Login(context.Background(), auth.Login{
		Username: "admin", Password: "correct horse battery staple",
	}); err != nil {
		t.Fatalf("seed Login: %v", err)
	}
	before := persistedSessionHash(t, db, 1)
	api := &failingCSRFAuthAPI{AuthAPI: service}
	handler := testAuthHandler(api, true)

	recorder := doJSONMutation(t, handler, http.MethodPost, "/api/v1/session",
		map[string]string{"username": "admin", "password": "correct horse battery staple"}, tokenString(0x31))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	if api.loginCalls != 0 {
		t.Errorf("Login calls = %d, want 0", api.loginCalls)
	}
	after := persistedSessionHash(t, db, 1)
	if !bytes.Equal(after, before) {
		t.Error("failed CSRF generation replaced the persisted session")
	}
}

func TestRedeemGeneratesOutgoingCSRFBeforePersistingAccountAndInviteClaim(t *testing.T) {
	db, service := newPersistentAuthService(t)
	if err := service.Bootstrap(context.Background(), auth.Bootstrap{
		Username: "admin", Password: "correct horse battery staple",
	}); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	invite, err := service.CreateInvite(context.Background(), 1)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	usersBefore := rowCount(t, db, "users")
	sessionsBefore := rowCount(t, db, "sessions")
	api := &failingCSRFAuthAPI{AuthAPI: service}
	handler := testAuthHandler(api, true)

	recorder := doJSONMutation(t, handler, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "new-member", "password": "correct horse battery staple", "invite_token": invite.Token,
	}, tokenString(0x32))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	if api.redeemCalls != 0 {
		t.Errorf("RedeemInvite calls = %d, want 0", api.redeemCalls)
	}
	if got := rowCount(t, db, "users"); got != usersBefore {
		t.Errorf("user count = %d, want unchanged %d", got, usersBefore)
	}
	if got := rowCount(t, db, "sessions"); got != sessionsBefore {
		t.Errorf("session count = %d, want unchanged %d", got, sessionsBefore)
	}
	var unredeemed bool
	if err := db.QueryRow("SELECT redeemed_by IS NULL FROM invites WHERE id = 1").Scan(&unredeemed); err != nil {
		t.Fatalf("read invite state: %v", err)
	}
	if !unredeemed {
		t.Error("failed CSRF generation claimed the invite")
	}
}

type failingCSRFAuthAPI struct {
	AuthAPI
	loginCalls  int
	redeemCalls int
}

func (a *failingCSRFAuthAPI) Login(ctx context.Context, command auth.Login) (auth.Session, error) {
	a.loginCalls++
	return a.AuthAPI.Login(ctx, command)
}

func (a *failingCSRFAuthAPI) RedeemInvite(ctx context.Context, command auth.CreateUser) (auth.Session, error) {
	a.redeemCalls++
	return a.AuthAPI.RedeemInvite(ctx, command)
}

func (a *failingCSRFAuthAPI) NewCSRFToken() (string, error) {
	return "", errors.New("test CSRF randomness failure")
}

func newPersistentAuthService(t *testing.T) (*sql.DB, *auth.Service) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	writer, err := store.NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})
	service, err := auth.NewService(store.NewAuthStore(db, writer),
		func() time.Time { return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC) },
		&sequenceReader{},
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return db, service
}

func persistedSessionHash(t *testing.T, db *sql.DB, userID int64) []byte {
	t.Helper()
	var tokenHash []byte
	if err := db.QueryRow("SELECT token_hash FROM sessions WHERE user_id = ?", userID).Scan(&tokenHash); err != nil {
		t.Fatalf("read persisted session: %v", err)
	}
	return tokenHash
}

func rowCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	query := map[string]string{
		"users": "SELECT count(*) FROM users", "sessions": "SELECT count(*) FROM sessions",
	}[table]
	if query == "" {
		t.Fatalf("unsupported test table %q", table)
	}
	var count int
	if err := db.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

type sequenceReader struct{ next byte }

func (r *sequenceReader) Read(destination []byte) (int, error) {
	for index := range destination {
		r.next++
		destination[index] = r.next
	}
	return len(destination), nil
}

var _ io.Reader = (*sequenceReader)(nil)
