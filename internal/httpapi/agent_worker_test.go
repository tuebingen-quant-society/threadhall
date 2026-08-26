package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

func TestAgentWorkerRequiresCanonicalBearerAndCompletesOwnedTask(t *testing.T) {
	t.Parallel()
	var raw [32]byte
	for index := range raw {
		raw[index] = 7
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	api := &fakeAgentWorkerAPI{work: agenttask.Work{Task: agenttask.Task{ID: 9, AgentID: 4}, Prompt: "bounded"}, found: true}
	mux := http.NewServeMux()
	RegisterAgentWorker(mux, api, nil)

	unauthorized := httptest.NewRecorder()
	mux.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/agent/work", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorized.Code)
	}

	claim := httptest.NewRequest(http.MethodGet, "/api/v1/agent/work", nil)
	claim.Header.Set("Authorization", "Bearer "+token)
	claimed := httptest.NewRecorder()
	mux.ServeHTTP(claimed, claim)
	if claimed.Code != http.StatusOK || !strings.Contains(claimed.Body.String(), `"prompt":"bounded"`) {
		t.Fatalf("claim = %d %s", claimed.Code, claimed.Body.String())
	}
	if claimed.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("claim Cache-Control = %q, want no-store", claimed.Header().Get("Cache-Control"))
	}
	if api.tokenHash != sha256.Sum256(raw[:]) {
		t.Fatal("handler did not hash the decoded bearer token")
	}

	complete := httptest.NewRequest(http.MethodPost, "/api/v1/agent/tasks/9/complete",
		strings.NewReader(`{"output":"## Done","runtime_thread_id":"019d"}`))
	complete.Header.Set("Authorization", "Bearer "+token)
	complete.Header.Set("Content-Type", "application/json")
	completed := httptest.NewRecorder()
	mux.ServeHTTP(completed, complete)
	if completed.Code != http.StatusNoContent || api.completion.TaskID != 9 || api.completion.Output != "## Done" {
		t.Fatalf("complete = %d %#v %s", completed.Code, api.completion, completed.Body.String())
	}
	fail := httptest.NewRequest(http.MethodPost, "/api/v1/agent/tasks/9/fail",
		strings.NewReader(`{"reason":"interaction_unsupported"}`))
	fail.Header.Set("Authorization", "Bearer "+token)
	fail.Header.Set("Content-Type", "application/json")
	failed := httptest.NewRecorder()
	mux.ServeHTTP(failed, fail)
	if failed.Code != http.StatusNoContent || api.failure.Reason != "interaction_unsupported" {
		t.Fatalf("fail = %d %#v %s", failed.Code, api.failure, failed.Body.String())
	}
}

func TestAgentWorkerRejectsUnknownAndOversizedCompletionFields(t *testing.T) {
	t.Parallel()
	raw := make([]byte, 32)
	token := base64.RawURLEncoding.EncodeToString(raw)
	api := &fakeAgentWorkerAPI{}
	mux := http.NewServeMux()
	RegisterAgentWorker(mux, api, nil)
	for _, body := range []string{
		`{"output":"ok","unknown":true}`,
		`{"output":"` + strings.Repeat("x", agenttask.MaxOutputBytes+1) + `"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/tasks/1/complete", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid completion status = %d, want 400", response.Code)
		}
	}
}

type fakeAgentWorkerAPI struct {
	work       agenttask.Work
	found      bool
	tokenHash  [32]byte
	completion agenttask.Completion
	failure    agenttask.Failure
}

func (f *fakeAgentWorkerAPI) Claim(_ context.Context, hash [32]byte, _ time.Time) (agenttask.Work, bool, error) {
	f.tokenHash = hash
	return f.work, f.found, nil
}

func (f *fakeAgentWorkerAPI) Progress(context.Context, [32]byte, int64, string, time.Time) error {
	return nil
}

func (f *fakeAgentWorkerAPI) Complete(_ context.Context, hash [32]byte, completion agenttask.Completion) error {
	f.tokenHash, f.completion = hash, completion
	return nil
}

func (f *fakeAgentWorkerAPI) Fail(_ context.Context, hash [32]byte, failure agenttask.Failure) error {
	f.tokenHash, f.failure = hash, failure
	return nil
}
