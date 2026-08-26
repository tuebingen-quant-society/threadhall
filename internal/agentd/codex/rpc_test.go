package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
)

func TestRunProtocolStartsFreshReadOnlyThreadAndReturnsMarkdown(t *testing.T) {
	t.Parallel()
	client, server := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = server.Close() })
	serverErr := make(chan error, 1)
	go func() { serverErr <- serveCodexFixture(server) }()

	result, err := runProtocol(context.Background(), client, "bounded prompt", "/tmp/empty")
	if err != nil {
		t.Fatalf("runProtocol: %v", err)
	}
	if result.ThreadID != "019d-thread" || result.Output != "## Answer\n\n```go\nfmt.Println(42)\n```" {
		t.Fatalf("result = %#v", result)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("fixture server: %v", err)
	}
}

func serveCodexFixture(connection net.Conn) error {
	scanner := bufio.NewScanner(connection)
	encoder := json.NewEncoder(connection)
	request, err := readRPCFixture(scanner)
	if err != nil || request.Method != "initialize" {
		return fixtureError("initialize", request, err)
	}
	if err := encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"userAgent": "codex-test"}}); err != nil {
		return err
	}
	notification, err := readRPCFixture(scanner)
	if err != nil || notification.Method != "initialized" {
		return fixtureError("initialized", notification, err)
	}
	request, err = readRPCFixture(scanner)
	if err != nil || request.Method != "thread/start" || request.Params["cwd"] != "/tmp/empty" ||
		request.Params["approvalPolicy"] != "never" || request.Params["sandbox"] != "read-only" || request.Params["ephemeral"] != true {
		return fixtureError("secure thread/start", request, err)
	}
	if err := encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{
		"thread": map[string]any{"id": "019d-thread"}, "model": "gpt-5", "modelProvider": "openai",
		"cwd": "/tmp/empty", "approvalPolicy": "never", "approvalsReviewer": "user", "sandbox": map[string]any{"type": "readOnly"},
	}}); err != nil {
		return err
	}
	request, err = readRPCFixture(scanner)
	if err != nil || request.Method != "turn/start" || request.Params["threadId"] != "019d-thread" {
		return fixtureError("turn/start", request, err)
	}
	input, _ := request.Params["input"].([]any)
	if len(input) != 1 || input[0].(map[string]any)["text"] != "bounded prompt" {
		return fixtureError("bounded turn input", request, nil)
	}
	if err := encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-1", "items": []any{}, "status": "inProgress"}}}); err != nil {
		return err
	}
	for _, delta := range []string{"## Answer\n\n", "```go\nfmt.Println(42)\n```"} {
		if err := encoder.Encode(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{
			"threadId": "019d-thread", "turnId": "turn-1", "itemId": "item-1", "delta": delta,
		}}); err != nil {
			return err
		}
	}
	return encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{
		"threadId": "019d-thread", "turn": map[string]any{"id": "turn-1", "items": []any{}, "status": "completed"},
	}})
}

type fixtureRequest struct {
	ID     int64
	Method string
	Params map[string]any
}

func readRPCFixture(scanner *bufio.Scanner) (fixtureRequest, error) {
	if !scanner.Scan() {
		return fixtureRequest{}, scanner.Err()
	}
	var raw struct {
		ID     int64          `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	err := json.Unmarshal(scanner.Bytes(), &raw)
	return fixtureRequest(raw), err
}

func fixtureError(want string, got fixtureRequest, err error) error {
	return &fixtureMismatch{want: want, got: got, err: err}
}

type fixtureMismatch struct {
	want string
	got  fixtureRequest
	err  error
}

func (e *fixtureMismatch) Error() string {
	encoded, _ := json.Marshal(e.got)
	return "want " + e.want + ", got " + string(encoded)
}
