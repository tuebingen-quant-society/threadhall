package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

const testOrigin = "https://threadhall.test"

func TestSessionGetSeedsStrictCSRFWithoutMutatingSession(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x41)}
	handler := testAuthHandler(api, true)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/session", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
	csrf := responseCookie(t, recorder, csrfCookieName)
	if csrf.Value != api.csrf || csrf.HttpOnly || !csrf.Secure || csrf.SameSite != http.SameSiteLaxMode || csrf.Path != "/" {
		t.Fatalf("CSRF cookie = %#v", csrf)
	}
	if api.authenticateCalls != 0 {
		t.Fatalf("Authenticate calls = %d, want 0 without a session cookie", api.authenticateCalls)
	}
}

func TestSessionGetAuthenticatesWithoutRotatingDatabaseState(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x42), user: auth.User{ID: 4, Username: "member"}}
	handler := testAuthHandler(api, false)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x23)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || api.authenticateCalls != 1 {
		t.Fatalf("GET session status/calls = %d/%d", recorder.Code, api.authenticateCalls)
	}
	if len(recorder.Result().Cookies()) != 1 {
		t.Fatalf("response cookies = %d, want only seeded CSRF", len(recorder.Result().Cookies()))
	}
	var body struct {
		User auth.User `json:"user"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil || body.User.ID != 4 {
		t.Fatalf("session response = (%#v, %v)", body, err)
	}
}

func TestMutationRequiresExactOriginAndDoubleSubmitCSRF(t *testing.T) {
	csrf := tokenString(0x51)
	api := &fakeAuthAPI{csrf: csrf, session: testSession()}
	handler := testAuthHandler(api, true)
	body := []byte(`{"username":"member","password":"correct horse battery staple"}`)
	tests := []struct {
		name, origin, header string
		want                 int
	}{
		{name: "missing origin", header: csrf, want: http.StatusForbidden},
		{name: "different origin", origin: "https://evil.test", header: csrf, want: http.StatusForbidden},
		{name: "missing header", origin: testOrigin, want: http.StatusForbidden},
		{name: "different header", origin: testOrigin, header: tokenString(0x52), want: http.StatusForbidden},
		{name: "matching values", origin: testOrigin, header: csrf, want: http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", test.origin)
			request.Header.Set(csrfHeaderName, test.header)
			request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}

func TestLoginSetsStrictSessionAndFreshCSRFCookies(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x61), session: testSession()}
	handler := testAuthHandler(api, true)
	recorder := doJSONMutation(t, handler, http.MethodPost, "/api/v1/session",
		map[string]string{"username": "member", "password": "correct horse battery staple"}, tokenString(0x31))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", recorder.Code, recorder.Body.String())
	}
	session := responseCookie(t, recorder, sessionCookieName)
	if session.Value != api.session.Token || !session.HttpOnly || !session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" {
		t.Fatalf("session cookie = %#v", session)
	}
	csrf := responseCookie(t, recorder, csrfCookieName)
	if csrf.Value != api.csrf || csrf.HttpOnly || !csrf.Secure || csrf.SameSite != http.SameSiteLaxMode {
		t.Fatalf("fresh CSRF cookie = %#v", csrf)
	}
}

func TestLoginDoesNotEmitActiveSessionCookieWhenCSRFGenerationFails(t *testing.T) {
	api := &fakeAuthAPI{csrfErr: errors.New("random source failed"), session: testSession()}
	handler := testAuthHandler(api, true)
	recorder := doJSONMutation(t, handler, http.MethodPost, "/api/v1/session",
		map[string]string{"username": "member", "password": "correct horse battery staple"}, tokenString(0x31))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			t.Fatalf("failed response emitted active session cookie %#v", cookie)
		}
	}
}

func TestLoginDoesNotRevealUsernameExistence(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x62), loginErr: auth.ErrInvalidCredentials}
	handler := testAuthHandler(api, false)
	var responses []string
	for _, username := range []string{"existing", "missing"} {
		recorder := doJSONMutation(t, handler, http.MethodPost, "/api/v1/session",
			map[string]string{"username": username, "password": "incorrect password"}, tokenString(0x32))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want 401", username, recorder.Code)
		}
		responses = append(responses, recorder.Body.String())
	}
	if responses[0] != responses[1] {
		t.Fatalf("credential failures differ:\n%s\n%s", responses[0], responses[1])
	}
}

func TestLoginMapsSaturatedPersistenceToStableUnavailableProblem(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x62), loginErr: auth.ErrBusy}
	handler := testAuthHandler(api, false)
	recorder := doJSONMutation(t, handler, http.MethodPost, "/api/v1/session",
		map[string]string{"username": "member", "password": "correct horse battery staple"}, tokenString(0x32))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", recorder.Code, recorder.Body.String())
	}
	var problem Problem
	if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Code != "temporarily_unavailable" {
		t.Fatalf("problem code = %q, want temporarily_unavailable", problem.Code)
	}
}

func TestInviteRedemptionAndLogoutUseNarrowPublicResponses(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x63), session: testSession(), user: testSession().User}
	handler := testAuthHandler(api, false)
	csrf := tokenString(0x33)
	redeem := doJSONMutation(t, handler, http.MethodPost, "/api/v1/users", map[string]string{
		"username": "new-member", "password": "correct horse battery staple", "invite_token": tokenString(0x15),
	}, csrf)
	if redeem.Code != http.StatusCreated {
		t.Fatalf("redeem status = %d, want 201; body=%s", redeem.Code, redeem.Body.String())
	}

	request := mutationRequest(http.MethodDelete, "/api/v1/session", nil, csrf)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x16)})
	logout := httptest.NewRecorder()
	handler.ServeHTTP(logout, request)
	if logout.Code != http.StatusNoContent || api.revokeCalls != 1 {
		t.Fatalf("logout status/revokes = %d/%d", logout.Code, api.revokeCalls)
	}
	cleared := responseCookie(t, logout, sessionCookieName)
	if cleared.MaxAge >= 0 || !cleared.HttpOnly || cleared.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cleared session cookie = %#v", cleared)
	}
}

func testAuthHandler(api AuthAPI, secure bool) http.Handler {
	mux := http.NewServeMux()
	RegisterAuth(mux, api, testOrigin, secure)
	return mux
}

func doJSONMutation(t *testing.T, handler http.Handler, method, path string, body any, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	request := mutationRequest(method, path, bytes.NewReader(encoded), csrf)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func mutationRequest(method, path string, body io.Reader, csrf string) *http.Request {
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Origin", testOrigin)
	request.Header.Set(csrfHeaderName, csrf)
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	return request
}

func responseCookie(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response has no %s cookie", name)
	return nil
}

func testSession() auth.Session {
	return auth.Session{
		Token: tokenString(0x71), ExpiresAt: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
		User: auth.User{ID: 4, Username: "member"},
	}
}

func tokenString(value byte) string {
	var raw [32]byte
	for index := range raw {
		raw[index] = value
	}
	return base64.RawURLEncoding.EncodeToString(raw[:])
}

type fakeAuthAPI struct {
	csrf              string
	user              auth.User
	session           auth.Session
	loginErr          error
	csrfErr           error
	inviteErr         error
	authenticateCalls int
	revokeCalls       int
}

func (a *fakeAuthAPI) Login(context.Context, auth.Login) (auth.Session, error) {
	return a.session, a.loginErr
}
func (a *fakeAuthAPI) RedeemInvite(context.Context, auth.CreateUser) (auth.Session, error) {
	return a.session, nil
}
func (a *fakeAuthAPI) CreateInvite(context.Context, int64) (auth.Invite, error) {
	return auth.Invite{}, a.inviteErr
}
func (a *fakeAuthAPI) Authenticate(context.Context, [32]byte) (auth.User, error) {
	a.authenticateCalls++
	if a.user.ID == 0 {
		return auth.User{}, auth.ErrUnauthenticated
	}
	return a.user, nil
}
func (a *fakeAuthAPI) Revoke(context.Context, [32]byte) error {
	a.revokeCalls++
	return nil
}
func (a *fakeAuthAPI) NewCSRFToken() (string, error) {
	if a.csrfErr != nil {
		return "", a.csrfErr
	}
	if a.csrf == "" {
		return "", errors.New("missing test CSRF")
	}
	return a.csrf, nil
}
