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

func TestDiscoverProtocolReturnsOnlyEnabledInstalledPluginsAndSkills(t *testing.T) {
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
		_, _ = readRPCFixture(scanner) // initialized
		request, err = readRPCFixture(scanner)
		if err != nil || request.Method != "plugin/list" {
			serverErr <- fixtureError("plugin/list", request, err)
			return
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"marketplaces": []any{map[string]any{
			"name": "local", "plugins": []any{
				map[string]any{"id": "drive@example", "name": "drive", "installed": true, "enabled": true, "interface": map[string]any{"displayName": "Drive", "shortDescription": "Files"}},
				map[string]any{"id": "off@example", "name": "off", "installed": false, "enabled": false},
			},
		}}}})
		request, err = readRPCFixture(scanner)
		if err != nil || request.Method != "skills/list" {
			serverErr <- fixtureError("skills/list", request, err)
			return
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"data": []any{map[string]any{
			"cwd": "/tmp/empty", "errors": []any{}, "skills": []any{
				map[string]any{"name": "better-layout", "description": "Layout", "enabled": true, "path": "/tmp/skill", "scope": "user"},
				map[string]any{"name": "off", "description": "Off", "enabled": false, "path": "/tmp/off", "scope": "user"},
			},
		}}}})
		serverErr <- nil
	}()

	items, err := discoverProtocol(context.Background(), client, "/tmp/empty")
	if err != nil {
		t.Fatalf("discoverProtocol: %v", err)
	}
	if len(items) != 2 || items[0].ID != "drive@example" || items[1].ID != "better-layout" {
		t.Fatalf("capabilities = %#v", items)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("fixture server: %v", err)
	}
}

func TestRunProtocolReadsCompletedMCPAppResources(t *testing.T) {
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
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"thread": map[string]any{"id": "app-thread"}}})
		request, err = readRPCFixture(scanner)
		if err != nil || request.Method != "turn/start" {
			serverErr <- fixtureError("turn/start", request, err)
			return
		}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-app", "status": "inProgress"}}})
		_ = encoder.Encode(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{
			"id": "tool-1", "type": "mcpToolCall", "server": "forms", "tool": "ask", "status": "completed",
			"arguments": map[string]any{"question": "Choose"}, "result": map[string]any{"content": []any{}},
			"appContext": map[string]any{"connectorId": "forms", "resourceUri": "ui://forms/ask"},
		}}})
		_ = encoder.Encode(map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"delta": "Please choose."}})
		_ = encoder.Encode(map[string]any{"method": "turn/completed", "params": map[string]any{"turn": map[string]any{"status": "completed"}}})
		request, err = readRPCFixture(scanner)
		if err != nil || request.Method != "mcpServer/resource/read" || request.Params["server"] != "forms" || request.Params["uri"] != "ui://forms/ask" {
			serverErr <- fixtureError("mcpServer/resource/read", request, err)
			return
		}
		contents := []any{map[string]any{
			"uri": "ui://forms/ask", "mimeType": "text/html;profile=mcp-app", "text": "<form><button>Choose</button></form>",
		}}
		_ = encoder.Encode(map[string]any{"id": request.ID, "result": map[string]any{"contents": contents}})
		serverErr <- nil
	}()

	result, err := runProtocol(context.Background(), client, "ask with a form", "/tmp/empty")
	if err != nil {
		t.Fatalf("runProtocol: %v", err)
	}
	if len(result.Apps) != 1 || result.Apps[0].ResourceURI != "ui://forms/ask" || result.Apps[0].HTML != "<form><button>Choose</button></form>" {
		t.Fatalf("apps = %#v", result.Apps)
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
