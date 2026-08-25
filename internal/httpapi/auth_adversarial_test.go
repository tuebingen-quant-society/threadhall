package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
)

func TestInviteCreationRequiresRepositoryConfirmedAdministrator(t *testing.T) {
	api := &fakeAuthAPI{
		csrf: tokenString(0x72), user: auth.User{ID: 4, Username: "member"}, inviteErr: auth.ErrForbidden,
	}
	handler := testAuthHandler(api, false)
	csrf := tokenString(0x34)
	request := mutationRequest(http.MethodPost, "/api/v1/invites", nil, csrf)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: tokenString(0x17)})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAuthenticationJSONIsBoundedAndRejectsUnknownFields(t *testing.T) {
	api := &fakeAuthAPI{csrf: tokenString(0x73), session: testSession()}
	handler := testAuthHandler(api, false)
	csrf := tokenString(0x35)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "unknown field", body: []byte(`{"username":"member","password":"correct horse battery staple","extra":true}`)},
		{name: "oversized body", body: bytes.Repeat([]byte{'x'}, maxAuthBodyBytes+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := mutationRequest(http.MethodPost, "/api/v1/session", bytes.NewReader(test.body), csrf)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
