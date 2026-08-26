package demomcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestServeExposesProjectBriefMCPApp(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"show_threadhall_project","arguments":{"focus":"agent collaboration"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"ui://threadhall/project-brief"}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	decoder := json.NewDecoder(&output)
	responses := make([]map[string]any, 0, 4)
	for decoder.More() {
		var response map[string]any
		if err := decoder.Decode(&response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		responses = append(responses, response)
	}
	if len(responses) != 4 {
		t.Fatalf("responses = %d, want 4: %s", len(responses), output.String())
	}
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	meta := tools[0].(map[string]any)["_meta"].(map[string]any)
	if meta["ui"].(map[string]any)["resourceUri"] != ResourceURI {
		t.Fatalf("tool metadata = %#v", meta)
	}
	annotations := tools[0].(map[string]any)["annotations"].(map[string]any)
	if annotations["readOnlyHint"] != true || annotations["destructiveHint"] != false || annotations["openWorldHint"] != false {
		t.Fatalf("tool annotations = %#v", annotations)
	}
	result := responses[2]["result"].(map[string]any)
	structured := result["structuredContent"].(map[string]any)
	if structured["focus"] != "agent collaboration" || result["isError"] != false {
		t.Fatalf("tool result = %#v", result)
	}
	contents := responses[3]["result"].(map[string]any)["contents"].([]any)
	content := contents[0].(map[string]any)
	if content["mimeType"] != AppMIME || !strings.Contains(content["text"].(string), "Threadhall project brief") {
		t.Fatalf("resource content = %#v", content)
	}
}

func TestServeRejectsUnknownMethodsAndTools(t *testing.T) {
	t.Parallel()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"unknown","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"missing","arguments":{}}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := Serve(strings.NewReader(input), &output); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if got := strings.Count(output.String(), `"code":-32601`); got != 2 {
		t.Fatalf("method errors = %d, want 2: %s", got, output.String())
	}
}
