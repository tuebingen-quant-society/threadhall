package agentd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

func TestClientClaimsAndCompletesWithoutSendingCredentialsInURLs(t *testing.T) {
	t.Parallel()
	var claimAuth, completeAuth, completePath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "GET /api/v1/agent/work":
			claimAuth = request.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(agenttask.Work{Task: agenttask.Task{ID: 7}, Prompt: "bounded"})
		case "POST /api/v1/agent/tasks/7/complete":
			completeAuth, completePath = request.Header.Get("Authorization"), request.URL.RequestURI()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, request)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "secret-worker-token", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	work, found, err := client.Next(context.Background())
	if err != nil || !found || work.Task.ID != 7 {
		t.Fatalf("Next = (%#v, %v, %v)", work, found, err)
	}
	if err := client.Complete(context.Background(), 7, "answer", "runtime-id"); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if claimAuth != "Bearer secret-worker-token" || completeAuth != claimAuth {
		t.Fatalf("authorization headers = %q/%q", claimAuth, completeAuth)
	}
	if completePath != "/api/v1/agent/tasks/7/complete" {
		t.Fatalf("completion URI = %q", completePath)
	}
}

func TestClientTreatsNoContentAsNoQueuedTask(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "token", server.Client())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, found, err := client.Next(context.Background()); err != nil || found {
		t.Fatalf("Next no-content = found %v, error %v", found, err)
	}
}
