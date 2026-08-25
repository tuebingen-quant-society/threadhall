package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
)

func TestMessageHTTPRoutesUseAuthenticatedActorAndServerRenderedResults(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 4, Username: "member"}}
	api := &fakeMessageAPI{result: message.Result{Message: message.Message{
		ID: 8, ConversationID: 3, AuthorID: 4, Body: "hello", RenderedBody: "<p>hello</p>\n",
	}}}
	handler := testMessageHandler(authAPI, api)
	csrf := tokenString(0x51)

	send := messageJSONMutation(t, handler, http.MethodPost, "/api/v1/conversations/3/messages",
		map[string]any{"body": "hello", "idempotency_key": "send-1"}, csrf, true)
	if send.Code != http.StatusCreated || api.sent.ConversationID != 3 || api.sent.AuthorID != 4 ||
		api.sent.Body != "hello" || send.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("send = status %d command %#v headers %#v; body=%s", send.Code, api.sent, send.Header(), send.Body.String())
	}
	var returned message.Result
	if err := json.NewDecoder(send.Body).Decode(&returned); err != nil || returned.Message.RenderedBody != "<p>hello</p>\n" {
		t.Fatalf("send result = (%#v, %v)", returned, err)
	}

	edit := messageJSONMutation(t, handler, http.MethodPatch, "/api/v1/messages/8",
		map[string]any{"body": "changed", "idempotency_key": "edit-1"}, csrf, true)
	if edit.Code != http.StatusOK || api.edited.MessageID != 8 || api.edited.AuthorID != 4 {
		t.Fatalf("edit = status %d command %#v; body=%s", edit.Code, api.edited, edit.Body.String())
	}
	deleted := messageJSONMutation(t, handler, http.MethodDelete, "/api/v1/messages/8",
		map[string]any{"idempotency_key": "delete-1"}, csrf, true)
	if deleted.Code != http.StatusOK || api.deleted.MessageID != 8 || api.deleted.AuthorID != 4 {
		t.Fatalf("delete = status %d command %#v; body=%s", deleted.Code, api.deleted, deleted.Body.String())
	}

	read := messageRead(t, handler, "/api/v1/conversations/3/messages?before_id=8&limit=2")
	if read.Code != http.StatusOK || api.history.ConversationID != 3 || api.history.UserID != 4 ||
		api.history.BeforeID != 8 || api.history.Limit != 2 {
		t.Fatalf("history = status %d query %#v; body=%s", read.Code, api.history, read.Body.String())
	}
}

func TestMessageHTTPSignalsCommittedEventWithoutChangingSuccess(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 4, Username: "member"}}
	api := &fakeMessageAPI{result: message.Result{
		Message: message.Message{ID: 8, ConversationID: 3, AuthorID: 4},
		Event:   realtime.Event{Seq: 44, Type: "message.sent", ConversationID: 3},
	}}
	notifier := &recordingNotifier{}
	mux := http.NewServeMux()
	RegisterMessages(mux, authAPI, api, notifier, testOrigin)
	csrf := tokenString(0x51)
	recorder := messageJSONMutation(t, mux, http.MethodPost, "/api/v1/conversations/3/messages",
		map[string]any{"body": "hello", "idempotency_key": "send"}, csrf, true)
	if recorder.Code != http.StatusCreated || notifier.sequence != 44 || notifier.calls != 1 {
		t.Fatalf("response/notifier = %d / %#v", recorder.Code, notifier)
	}
}

func TestMessageHTTPRejectsClientHTMLInvalidUTF8AndOversizedBodiesBeforeAPI(t *testing.T) {
	authAPI := &fakeAuthAPI{user: auth.User{ID: 4, Username: "member"}}
	api := &fakeMessageAPI{}
	handler := testMessageHandler(authAPI, api)
	csrf := tokenString(0x54)

	for _, test := range []struct {
		name, code string
		body       any
		status     int
	}{
		{name: "client-rendered-html", status: 400, code: "invalid_request", body: map[string]any{
			"body": "hello", "rendered_body": "<script>pwn()</script>", "idempotency_key": "send",
		}},
		{name: "oversized-body", status: 413, code: "request_too_large", body: map[string]any{
			"body": strings.Repeat("a", message.MaxBodyBytes+1), "idempotency_key": "send",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := messageJSONMutation(t, handler, http.MethodPost,
				"/api/v1/conversations/3/messages", test.body, csrf, true)
			assertMessageProblem(t, recorder, test.status, test.code)
		})
	}
	invalid := []byte(`{"body":"`)
	invalid = append(invalid, 0xff)
	invalid = append(invalid, []byte(`","idempotency_key":"send"}`)...)
	request := mutationRequest(http.MethodPost, "/api/v1/conversations/3/messages", bytes.NewReader(invalid), csrf)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x55)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	assertMessageProblem(t, recorder, http.StatusBadRequest, "invalid_request")
	if api.calls != 0 {
		t.Fatalf("message API calls = %d, want 0", api.calls)
	}
}

func TestMessageHTTPAppliesTargetGuardsBeforeAuthenticationAndSecurity(t *testing.T) {
	handler := testMessageHandler(&fakeAuthAPI{}, &fakeMessageAPI{})
	for _, target := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/conversations/3/messages?before_id=nope"},
		{http.MethodGet, "/api/v1/conversations/3/messages?limit=101"},
		{http.MethodGet, "/api/v1/conversations/3/messages?unknown=1"},
		{http.MethodPost, "/api/v1/conversations/3/messages?unexpected=1"},
		{http.MethodPatch, "/api/v1/messages/8?unexpected=1"},
		{http.MethodDelete, "/api/v1/messages/8?unexpected=1"},
	} {
		request := httptest.NewRequest(target.method, target.path, nil)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		assertMessageProblem(t, recorder, http.StatusBadRequest, "invalid_request")
	}
	oversized := httptest.NewRequest(http.MethodGet,
		"/api/v1/conversations/3/messages?limit="+strings.Repeat("0", 2049)+"1", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, oversized)
	assertMessageProblem(t, recorder, http.StatusBadRequest, "invalid_request")
}

func TestMessageHTTPRequiresMutationSecurityAndUsesGenericMissingProblem(t *testing.T) {
	csrf := tokenString(0x56)
	authAPI := &fakeAuthAPI{user: auth.User{ID: 4, Username: "member"}}
	handler := testMessageHandler(authAPI, &fakeMessageAPI{err: message.ErrNotFound})

	unauthenticated := messageJSONMutation(t, handler, http.MethodPost, "/api/v1/conversations/3/messages",
		map[string]any{"body": "hello", "idempotency_key": "send"}, csrf, false)
	assertMessageProblem(t, unauthenticated, http.StatusUnauthorized, "authentication_required")
	missingCSRF := messageJSONMutation(t, handler, http.MethodPatch, "/api/v1/messages/8",
		map[string]any{"body": "changed", "idempotency_key": "edit"}, "", true)
	assertMessageProblem(t, missingCSRF, http.StatusForbidden, "csrf_invalid")

	missingHistory := messageRead(t, handler, "/api/v1/conversations/3/messages")
	missingEdit := messageJSONMutation(t, handler, http.MethodPatch, "/api/v1/messages/8",
		map[string]any{"body": "changed", "idempotency_key": "edit"}, csrf, true)
	assertMessageProblem(t, missingHistory, http.StatusNotFound, "not_found")
	assertMessageProblem(t, missingEdit, http.StatusNotFound, "not_found")
	if missingHistory.Body.String() != missingEdit.Body.String() {
		t.Fatalf("missing and inaccessible responses differ:\n%s\n%s", missingHistory.Body.String(), missingEdit.Body.String())
	}
}

func assertMessageProblem(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var problem Problem
	if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v; body=%s", err, recorder.Body.String())
	}
	if recorder.Code != status || problem.Code != code || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("problem = status %d body %#v headers %#v", recorder.Code, problem, recorder.Header())
	}
}

func testMessageHandler(authAPI AuthAPI, api MessageAPI) http.Handler {
	mux := http.NewServeMux()
	RegisterMessages(mux, authAPI, api, &recordingNotifier{}, testOrigin)
	return mux
}

func messageJSONMutation(
	t *testing.T,
	handler http.Handler,
	method, path string,
	body any,
	csrf string,
	session bool,
) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal message request: %v", err)
	}
	request := mutationRequest(method, path, bytes.NewReader(encoded), csrf)
	request.Header.Set("Content-Type", "application/json")
	if session {
		request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x52)})
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func messageRead(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x53)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

type fakeMessageAPI struct {
	sent    message.Send
	edited  message.Edit
	deleted message.Delete
	history message.History
	result  message.Result
	err     error
	calls   int
}

type recordingNotifier struct {
	sequence int64
	calls    int
}

func (n *recordingNotifier) Notify(sequence int64) {
	n.sequence = sequence
	n.calls++
}

func (a *fakeMessageAPI) Send(_ context.Context, command message.Send) (message.Result, error) {
	a.calls++
	a.sent = command
	return a.result, a.err
}

func (a *fakeMessageAPI) Edit(_ context.Context, command message.Edit) (message.Result, error) {
	a.calls++
	a.edited = command
	return a.result, a.err
}

func (a *fakeMessageAPI) Delete(_ context.Context, command message.Delete) (message.Result, error) {
	a.calls++
	a.deleted = command
	return a.result, a.err
}

func (a *fakeMessageAPI) History(_ context.Context, query message.History) (message.Page, error) {
	a.calls++
	a.history = query
	return message.Page{}, a.err
}
