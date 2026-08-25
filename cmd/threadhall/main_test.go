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
