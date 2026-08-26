package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
)

func TestRunProtocolCapturesInteractiveQuestionAsDurableCard(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	serverErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(server)
		encoder := json.NewEncoder(server)
		request, err := readRPCFixture(scanner)
		if err != nil || request.Method != "initialize" {
			serverErr <- fixtureError("initialize", request, err)
			return
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"userAgent": "codex-test"}})
		_, _ = readRPCFixture(scanner)
		request, err = readRPCFixture(scanner)
		if err != nil || request.Method != "thread/start" {
			serverErr <- fixtureError("thread/start", request, err)
			return
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"thread": map[string]any{"id": "question-thread"}}})
		request, err = readRPCFixture(scanner)
		if err != nil || request.Method != "turn/start" {
			serverErr <- fixtureError("turn/start", request, err)
			return
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-question", "status": "inProgress"}}})
		_ = encoder.Encode(map[string]any{"id": 77, "method": "item/tool/requestUserInput", "params": map[string]any{
			"threadId": "question-thread", "turnId": "turn-question", "itemId": "item-question",
			"questions": []any{map[string]any{
				"id": "scope", "header": "Scope", "question": "Which scope should I implement?", "isOther": true,
				"options": []any{
					map[string]any{"label": "Channel", "description": "Only the current channel."},
					map[string]any{"label": "Workspace", "description": "Every project channel."},
				},
			}},
		}})
		response, err := readRPCFixture(scanner)
		if err != nil || response.ID != 77 {
			serverErr <- fixtureError("question capture response", response, err)
			return
		}
		serverErr <- nil
	}()

	result, err := runProtocol(context.Background(), client, "refine scope", "/tmp/empty", threadConfig{})
	if err != nil {
		t.Fatalf("runProtocol: %v", err)
	}
	if len(result.Questions) != 1 || result.Questions[0].ID != "scope" || len(result.Questions[0].Options) != 2 ||
		result.Questions[0].Options[0].Label != "Channel" {
		t.Fatalf("questions = %#v", result.Questions)
	}
	if result.Output == "" {
		t.Fatal("question output is empty")
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("fixture server: %v", err)
	}
}
