package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func TestConversationHTTPPreventsPrivateAndDirectMessageIDOR(t *testing.T) {
	service := newHTTPConversationService(t)
	private, err := service.CreateChannel(context.Background(), conversation.CreateChannel{
		CreatorID: 1, Kind: conversation.KindPrivate, Name: "private",
		IdempotencyKey: "private",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	dm, err := service.CreateDM(context.Background(), conversation.CreateDM{
		RequesterID: 1, OtherUserID: 2, IdempotencyKey: "dm-one-two",
	})
	if err != nil {
		t.Fatalf("CreateDM: %v", err)
	}
	outsider := testConversationHandler(&fakeAuthAPI{user: auth.User{ID: 3, Username: "outsider"}}, service)

	list := conversationRead(t, outsider, "/api/v1/conversations")
	var page conversation.ConversationPage
	if err := json.NewDecoder(list.Body).Decode(&page); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Code != http.StatusOK || len(page.Conversations) != 0 {
		t.Fatalf("outsider list = status %d page %#v", list.Code, page)
	}

	var failures []string
	for _, path := range []string{
		"/api/v1/conversations/" + idString(private.ID),
		"/api/v1/conversations/" + idString(dm.ID),
		"/api/v1/conversations/999999",
		"/api/v1/conversations/" + idString(private.ID) + "/members",
		"/api/v1/conversations/" + idString(dm.ID) + "/members",
	} {
		recorder := conversationRead(t, outsider, path)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404; body=%s", path, recorder.Code, recorder.Body.String())
		}
		failures = append(failures, recorder.Body.String())
	}
	for _, body := range failures[1:] {
		if body != failures[0] {
			t.Fatalf("inaccessible resources returned distinguishable problems:\n%q\n%q", failures[0], body)
		}
	}
}

func TestConversationHTTPRunsAdminMembershipAndCanonicalDMFlows(t *testing.T) {
	service := newHTTPConversationService(t)
	admin := testConversationHandler(&fakeAuthAPI{user: auth.User{ID: 1, Username: "admin", Admin: true}}, service)
	member := testConversationHandler(&fakeAuthAPI{user: auth.User{ID: 2, Username: "member"}}, service)
	csrf := tokenString(0x45)
	created := conversationJSONMutation(t, admin, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "channel", "name": "General", "idempotency_key": "create-general",
	}, csrf, true)
	if created.Code != http.StatusCreated {
		t.Fatalf("create channel status = %d; body=%s", created.Code, created.Body.String())
	}
	var channel conversation.Conversation
	if err := json.NewDecoder(created.Body).Decode(&channel); err != nil {
		t.Fatalf("decode channel: %v", err)
	}
	add := conversationJSONMutation(t, admin, http.MethodPost,
		"/api/v1/conversations/"+idString(channel.ID)+"/members", map[string]any{
			"user_id": 2, "idempotency_key": "add-member-two",
		}, csrf, true)
	if add.Code != http.StatusNoContent {
		t.Fatalf("admin add status = %d; body=%s", add.Code, add.Body.String())
	}
	if detail := conversationRead(t, member, "/api/v1/conversations/"+idString(channel.ID)); detail.Code != http.StatusOK {
		t.Fatalf("added member detail status = %d", detail.Code)
	}
	forbidden := conversationJSONMutation(t, member, http.MethodPost,
		"/api/v1/conversations/"+idString(channel.ID)+"/members", map[string]any{
			"user_id": 3, "idempotency_key": "ordinary-add",
		}, csrf, true)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("ordinary add status = %d, want 403", forbidden.Code)
	}

	dmOne := conversationJSONMutation(t, admin, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "dm", "other_user_id": 2, "idempotency_key": "admin-member-dm",
	}, csrf, true)
	dmTwo := conversationJSONMutation(t, member, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "dm", "other_user_id": 1, "idempotency_key": "member-admin-dm",
	}, csrf, true)
	var firstDM, secondDM conversation.Conversation
	if dmOne.Code != 201 || dmTwo.Code != 201 || json.NewDecoder(dmOne.Body).Decode(&firstDM) != nil ||
		json.NewDecoder(dmTwo.Body).Decode(&secondDM) != nil || firstDM.ID != secondDM.ID {
		t.Fatalf("canonical DM responses = %d/%d %#v/%#v", dmOne.Code, dmTwo.Code, firstDM, secondDM)
	}
	dmMember := conversationJSONMutation(t, admin, http.MethodPost,
		"/api/v1/conversations/"+idString(firstDM.ID)+"/members", map[string]any{
			"user_id": 3, "idempotency_key": "dm-add",
		}, csrf, true)
	if dmMember.Code != http.StatusNotFound {
		t.Fatalf("DM member-management status = %d, want 404", dmMember.Code)
	}
	duplicateName := conversationJSONMutation(t, admin, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "private", "name": "general", "idempotency_key": "duplicate-name",
	}, csrf, true)
	if duplicateName.Code != http.StatusConflict {
		t.Fatalf("duplicate-name status = %d, want 409", duplicateName.Code)
	}
	if page := conversationRead(t, admin, "/api/v1/conversations?limit=101"); page.Code != http.StatusBadRequest {
		t.Fatalf("oversized page status = %d, want 400", page.Code)
	}
}

func newHTTPConversationService(t *testing.T) *conversation.Service {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writer, err := store.NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
		_ = db.Close()
	})
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	if err := writer.Do(context.Background(), func(tx *sql.Tx) error {
		for id, username := range []string{"admin", "member", "outsider"} {
			_, err := tx.Exec(`INSERT INTO users(id, username, password_hash, is_admin, created_at)
				VALUES (?, ?, 'hash', ?, ?)`, id+1, username, id == 0, now.Unix())
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	service, err := conversation.NewService(store.NewConversationStore(db, writer), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func conversationRead(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x22)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func idString(id int64) string { return strconv.FormatInt(id, 10) }
