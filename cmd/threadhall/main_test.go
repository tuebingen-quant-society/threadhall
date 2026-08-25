package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/auth"
	"github.com/tuebingen-quant-society/threadhall/internal/config"
	"github.com/tuebingen-quant-society/threadhall/internal/httpapi"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func TestProductionHandlerRejectsInvalidConversationTargetsBeforeSecurity(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writer, err := store.NewWriter(db, 2)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
		_ = db.Close()
	})
	handler, err := newServerHandler(db, writer, config.Config{PublicURL: "https://threadhall.test"})
	if err != nil {
		t.Fatalf("newServerHandler: %v", err)
	}
	for _, target := range []struct {
		method, path, rawQuery string
	}{
		{http.MethodGet, "/api/v1/conversations", "limit=" + strings.Repeat("0", 2049) + "1"},
		{http.MethodGet, "/api/v1/conversations", "before_id=nope"},
		{http.MethodPost, "/api/v1/conversations", "unexpected=1"},
		{http.MethodGet, "/api/v1/conversations/1/messages", "before_id=nope"},
		{http.MethodPost, "/api/v1/conversations/1/messages", "unexpected=1"},
	} {
		request := httptest.NewRequest(target.method, target.path, nil)
		request.URL.RawQuery = target.rawQuery
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		var problem httpapi.Problem
		if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
			t.Fatalf("decode %s %s problem: %v", target.method, target.rawQuery, err)
		}
		if recorder.Code != http.StatusBadRequest || problem.Code != "invalid_request" {
			t.Fatalf("%s %s = status %d problem %#v", target.method, target.rawQuery, recorder.Code, problem)
		}
	}
}

func TestProductionHandlerRunsMessagePreflightBeforeSecurity(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 1)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	writer, err := store.NewWriter(db, 2)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = db.Close() })
	handler, err := newServerHandler(db, writer, config.Config{PublicURL: "https://threadhall.test"})
	if err != nil {
		t.Fatalf("newServerHandler: %v", err)
	}
	invalidUTF8 := append([]byte(`{"body":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","idempotency_key":"edit"}`)...)
	for _, test := range []struct {
		name, method, path, contentType string
		body                            []byte
		status                          int
		code                            string
	}{
		{name: "history id", method: http.MethodGet, path: "/api/v1/conversations/nope/messages", status: 400, code: "invalid_request"},
		{name: "send id", method: http.MethodPost, path: "/api/v1/conversations/0/messages", contentType: "application/json", body: []byte(`{"body":"hello","idempotency_key":"send"}`), status: 400, code: "invalid_request"},
		{name: "content type", method: http.MethodPost, path: "/api/v1/conversations/1/messages", contentType: "text/plain", body: []byte(`{"body":"hello","idempotency_key":"send"}`), status: 400, code: "invalid_request"},
		{name: "oversized body", method: http.MethodPost, path: "/api/v1/conversations/1/messages", contentType: "application/json", body: mustJSON(t, map[string]any{"body": strings.Repeat("a", message.MaxBodyBytes+1), "idempotency_key": "send"}), status: 413, code: "request_too_large"},
		{name: "invalid UTF-8", method: http.MethodPatch, path: "/api/v1/messages/1", contentType: "application/json", body: invalidUTF8, status: 400, code: "invalid_request"},
		{name: "unknown field", method: http.MethodPatch, path: "/api/v1/messages/1", contentType: "application/json", body: []byte(`{"body":"hello","idempotency_key":"edit","unknown":true}`), status: 400, code: "invalid_request"},
		{name: "trailing object", method: http.MethodPatch, path: "/api/v1/messages/1", contentType: "application/json", body: []byte(`{"body":"hello","idempotency_key":"edit"}{}`), status: 400, code: "invalid_request"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			assertProductionProblem(t, recorder, test.status, test.code)
		})
	}
	valid := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/messages",
		bytes.NewReader([]byte(`{"body":"hello","idempotency_key":"send"}`)))
	valid.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, valid)
	assertProductionProblem(t, recorder, http.StatusForbidden, "origin_forbidden")
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return encoded
}

func assertProductionProblem(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var problem httpapi.Problem
	if err := json.NewDecoder(recorder.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v; body=%s", err, recorder.Body.String())
	}
	if recorder.Code != status || problem.Code != code || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("problem = status %d body %#v headers %#v", recorder.Code, problem, recorder.Header())
	}
}

func TestRunPreservesVersionCommand(t *testing.T) {
	input := emptyInput(t)
	var output bytes.Buffer
	if err := run([]string{"version"}, input, &output); err != nil {
		t.Fatalf("run version: %v", err)
	}
	if output.String() != "dev\n" {
		t.Fatalf("version output = %q, want %q", output.String(), "dev\\n")
	}
}

func TestBootstrapAdminReadsPasswordFromStdinAndNeverAFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threadhall.db")
	input := pipeInput(t, "correct horse battery staple\n")
	var output bytes.Buffer
	if err := run([]string{"bootstrap-admin", "--state-path", path, "--username", "admin"}, input, &output); err != nil {
		t.Fatalf("bootstrap-admin: %v", err)
	}

	db, err := store.Open(path, 1)
	if err != nil {
		t.Fatalf("open bootstrapped database: %v", err)
	}
	defer db.Close()
	var passwordHash string
	if err := db.QueryRow("SELECT password_hash FROM users WHERE username = 'admin' AND is_admin = 1").Scan(&passwordHash); err != nil {
		t.Fatalf("read administrator: %v", err)
	}
	if passwordHash == "correct horse battery staple" || !auth.VerifyPassword("correct horse battery staple", passwordHash) {
		t.Fatal("bootstrap did not persist a valid password hash")
	}

	rejectedPath := filepath.Join(t.TempDir(), "rejected.db")
	err = run([]string{"bootstrap-admin", "--state-path", rejectedPath, "--username", "admin", "--password", "secret"}, emptyInput(t), &output)
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("password flag error = %v, want unknown-flag rejection", err)
	}
	if _, statErr := os.Stat(rejectedPath); !os.IsNotExist(statErr) {
		t.Fatalf("rejected password flag created state file: %v", statErr)
	}
}

func TestReadPasswordRejectsMultipleLinesAndOversizedInput(t *testing.T) {
	for _, input := range []string{
		"first valid password\nsecond valid password\n",
		strings.Repeat("p", 130),
	} {
		if _, err := readPassword(pipeInput(t, input), &bytes.Buffer{}); err == nil {
			t.Fatalf("readPassword(%d bytes) error = nil, want bounded single-line rejection", len(input))
		}
	}
}

func pipeInput(t *testing.T, value string) *os.File {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create input pipe: %v", err)
	}
	if _, err := writer.WriteString(value); err != nil {
		t.Fatalf("write input pipe: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close input writer: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	return reader
}

func emptyInput(t *testing.T) *os.File { return pipeInput(t, "") }
