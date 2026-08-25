package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestMessageHTTPPreflightRejectsMalformedBoundariesBeforeSecurity(t *testing.T) {
	authAPI := &fakeAuthAPI{}
	api := &fakeMessageAPI{}
	handler := testMessageHandler(authAPI, api)
	valid, err := json.Marshal(map[string]any{"body": "hello", "idempotency_key": "send"})
	if err != nil {
		t.Fatalf("marshal valid body: %v", err)
	}
	oversizedBody, err := json.Marshal(map[string]any{
		"body": strings.Repeat("a", message.MaxBodyBytes+1), "idempotency_key": "send",
	})
	if err != nil {
		t.Fatalf("marshal oversized body: %v", err)
	}
	invalidUTF8 := append([]byte(`{"body":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","idempotency_key":"send"}`)...)

	tests := []struct {
		name, method, path, contentType string
		body                            []byte
		status                          int
		code                            string
		unknownLength                   bool
	}{
		{name: "history id", method: http.MethodGet, path: "/api/v1/conversations/nope/messages", status: 400, code: "invalid_request"},
		{name: "send id", method: http.MethodPost, path: "/api/v1/conversations/0/messages", contentType: "application/json", body: valid, status: 400, code: "invalid_request"},
		{name: "edit id", method: http.MethodPatch, path: "/api/v1/messages/nope", contentType: "application/json", body: valid, status: 400, code: "invalid_request"},
		{name: "delete id", method: http.MethodDelete, path: "/api/v1/messages/0", status: 400, code: "invalid_request"},
		{name: "wrong content type", method: http.MethodPost, path: "/api/v1/conversations/1/messages", contentType: "text/plain", body: valid, status: 400, code: "invalid_request"},
		{name: "decoded body too large", method: http.MethodPatch, path: "/api/v1/messages/1", contentType: "application/json", body: oversizedBody, status: 413, code: "request_too_large"},
		{name: "stream too large", method: http.MethodPost, path: "/api/v1/conversations/1/messages", contentType: "application/json", body: bytes.Repeat([]byte("x"), maxMessageRequestBytes+1), status: 413, code: "request_too_large", unknownLength: true},
		{name: "invalid UTF-8", method: http.MethodPost, path: "/api/v1/conversations/1/messages", contentType: "application/json", body: invalidUTF8, status: 400, code: "invalid_request"},
		{name: "unknown field", method: http.MethodPost, path: "/api/v1/conversations/1/messages", contentType: "application/json", body: []byte(`{"body":"hello","idempotency_key":"send","rendered_body":"owned"}`), status: 400, code: "invalid_request"},
		{name: "trailing object", method: http.MethodPatch, path: "/api/v1/messages/1", contentType: "application/json", body: []byte(`{"body":"hello","idempotency_key":"edit"}{}`), status: 400, code: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewReader(test.body))
			if test.unknownLength {
				request.ContentLength = -1
			}
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assertMessageProblem(t, recorder, test.status, test.code)
		})
	}
	malformed := []byte(`{"body":"hello","idempotency_key":"send","unknown":true}`)
	for _, security := range []struct {
		name      string
		origin    bool
		validCSRF bool
	}{
		{name: "before origin"},
		{name: "before csrf", origin: true},
		{name: "before session", origin: true, validCSRF: true},
	} {
		t.Run(security.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/messages", bytes.NewReader(malformed))
			request.Header.Set("Content-Type", "application/json")
			if security.origin {
				request.Header.Set("Origin", testOrigin)
			}
			if security.validCSRF {
				csrf := tokenString(0x59)
				request.Header.Set(csrfHeaderName, csrf)
				request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assertMessageProblem(t, recorder, http.StatusBadRequest, "invalid_request")
		})
	}
	if authAPI.authenticateCalls != 0 || api.calls != 0 {
		t.Fatalf("preflight invoked auth/API = %d/%d, want 0/0", authAPI.authenticateCalls, api.calls)
	}
}

func TestMessageHTTPPreflightLetsValidInputReachSecurity(t *testing.T) {
	authAPI := &fakeAuthAPI{}
	api := &fakeMessageAPI{}
	handler := testMessageHandler(authAPI, api)
	valid := map[string]any{"body": "hello", "idempotency_key": "send"}

	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("marshal valid body: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/messages", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	missingOrigin := httptest.NewRecorder()
	handler.ServeHTTP(missingOrigin, request)
	assertMessageProblem(t, missingOrigin, http.StatusForbidden, "origin_forbidden")
	if request.Body != http.NoBody {
		t.Fatal("preflight did not replace the consumed request body")
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/messages", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", testOrigin)
	missingCSRF := httptest.NewRecorder()
	handler.ServeHTTP(missingCSRF, request)
	assertMessageProblem(t, missingCSRF, http.StatusForbidden, "csrf_invalid")
	request = httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/messages", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", testOrigin)
	csrf := tokenString(0x5a)
	request.Header.Set(csrfHeaderName, csrf)
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	missingSession := httptest.NewRecorder()
	handler.ServeHTTP(missingSession, request)
	assertMessageProblem(t, missingSession, http.StatusUnauthorized, "authentication_required")
	validRead := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/1/messages?limit=2", nil)
	readRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readRecorder, validRead)
	assertMessageProblem(t, readRecorder, http.StatusUnauthorized, "authentication_required")
	if api.calls != 0 {
		t.Fatalf("message API calls = %d, want 0", api.calls)
	}
}
