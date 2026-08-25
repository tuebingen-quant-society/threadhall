package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	"github.com/tuebingen-quant-society/threadhall/internal/conversation"
)

func TestConversationMutationsRequireSessionCSRFAndIdempotencyKeys(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 4, Username: "member"}, csrf: tokenString(0x41)}
	api := &fakeConversationAPI{created: conversation.Conversation{ID: 9, Kind: conversation.KindPrivate, Name: "staff"}}
	handler := testConversationHandler(authAPI, api)
	csrf := tokenString(0x42)

	valid := conversationJSONMutation(t, handler, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "private", "name": " staff ", "idempotency_key": "create-staff",
	}, csrf, true)
	if valid.Code != http.StatusCreated || api.channel.CreatorID != 4 || api.channel.IdempotencyKey != "create-staff" {
		t.Fatalf("valid create = status %d command %#v; body=%s", valid.Code, api.channel, valid.Body.String())
	}

	missingCSRF := conversationJSONMutation(t, handler, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "channel", "name": "general", "idempotency_key": "general",
	}, "", true)
	if missingCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing-CSRF status = %d, want 403", missingCSRF.Code)
	}
	unauthenticated := conversationJSONMutation(t, handler, http.MethodPost, "/api/v1/conversations", map[string]any{
		"kind": "channel", "name": "general", "idempotency_key": "general",
	}, csrf, false)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", unauthenticated.Code)
	}
}

func TestConversationHTTPMapsStableDomainProblems(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 1, Username: "admin"}, csrf: tokenString(0x43)}
	csrf := tokenString(0x44)
	for _, test := range []struct {
		name string
		err  error
		want int
		code string
	}{
		{name: "invalid", err: conversation.ErrInvalidInput, want: 400, code: "invalid_request"},
		{name: "missing", err: conversation.ErrNotFound, want: 404, code: "not_found"},
		{name: "conflict", err: conversation.ErrConflict, want: 409, code: "conversation_conflict"},
		{name: "forbidden", err: conversation.ErrForbidden, want: 403, code: "membership_admin_required"},
		{name: "busy", err: conversation.ErrBusy, want: 503, code: "temporarily_unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			api := &fakeConversationAPI{createErr: test.err}
			recorder := conversationJSONMutation(t, testConversationHandler(authAPI, api), http.MethodPost,
				"/api/v1/conversations", map[string]any{
					"kind": "channel", "name": "general", "idempotency_key": "general",
				}, csrf, true)
			var problem Problem
			if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if recorder.Code != test.want || problem.Code != test.code {
				t.Fatalf("problem = status %d body %#v", recorder.Code, problem)
			}
		})
	}
}

func testConversationHandler(authAPI AuthAPI, api ConversationAPI) http.Handler {
	mux := http.NewServeMux()
	RegisterConversations(mux, authAPI, api, testOrigin)
	return mux
}

func conversationJSONMutation(t *testing.T, handler http.Handler, method, path string, body any, csrf string, session bool) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	request := mutationRequest(method, path, bytes.NewReader(encoded), csrf)
	request.Header.Set("Content-Type", "application/json")
	if session {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x21)})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

type fakeConversationAPI struct {
	channel   conversation.CreateChannel
	dm        conversation.CreateDM
	member    conversation.ChangeMember
	created   conversation.Conversation
	createErr error
}

func (a *fakeConversationAPI) CreateChannel(_ context.Context, command conversation.CreateChannel) (conversation.Conversation, error) {
	a.channel = command
	return a.created, a.createErr
}
func (a *fakeConversationAPI) CreateDM(_ context.Context, command conversation.CreateDM) (conversation.Conversation, error) {
	a.dm = command
	return a.created, a.createErr
}
func (a *fakeConversationAPI) List(context.Context, conversation.ListConversations) (conversation.ConversationPage, error) {
	return conversation.ConversationPage{}, nil
}
func (a *fakeConversationAPI) Detail(context.Context, int64, int64) (conversation.Conversation, error) {
	return a.created, nil
}
func (a *fakeConversationAPI) Members(context.Context, conversation.ListMembers) (conversation.MemberPage, error) {
	return conversation.MemberPage{}, nil
}
func (a *fakeConversationAPI) AddMember(_ context.Context, command conversation.ChangeMember) error {
	a.member = command
	return a.createErr
}
func (a *fakeConversationAPI) RemoveMember(_ context.Context, command conversation.ChangeMember) error {
	a.member = command
	return a.createErr
}
