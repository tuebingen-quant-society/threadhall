package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func TestHealthReflectsDatabaseAvailability(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "threadhall.db"), 1)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	handler := New(db)

	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("healthy status = %d, want %d", recorder.Code, http.StatusOK)
	}

	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed database status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestUnknownAPIPathDoesNotServeApplicationShell(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/unknown", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	recorder := httptest.NewRecorder()

	New(nil).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}
