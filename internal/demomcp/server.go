package demomcp

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"io"
	"strings"
)

const (
	ResourceURI = "ui://threadhall/project-brief"
	AppMIME     = "text/html;profile=mcp-app"
	maxFrame    = 1 << 20
)

//go:embed app.html
var appHTML string

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func Serve(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), maxFrame)
	encoder := json.NewEncoder(output)
	for scanner.Scan() {
		var frame request
		if json.Unmarshal(scanner.Bytes(), &frame) != nil || frame.JSONRPC != "2.0" || frame.Method == "" {
			continue
		}
		if len(frame.ID) == 0 {
			continue
		}
		result, rpcError := dispatch(frame)
		response := map[string]any{"jsonrpc": "2.0", "id": frame.ID}
		if rpcError != nil {
			response["error"] = rpcError
		} else {
			response["result"] = result
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func dispatch(frame request) (any, any) {
	switch frame.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(frame.Params, &params)
		if params.ProtocolVersion == "" {
			params.ProtocolVersion = "2025-11-25"
		}
		return map[string]any{
			"protocolVersion": params.ProtocolVersion,
			"serverInfo":      map[string]any{"name": "threadhall-ui-demo", "version": "0.1.0"},
			"capabilities":    map[string]any{"tools": map[string]any{}, "resources": map[string]any{}},
		}, nil
	case "tools/list":
		return map[string]any{"tools": []any{projectTool()}}, nil
	case "tools/call":
		return callTool(frame.Params)
	case "resources/list":
		return map[string]any{"resources": []any{projectResource(false)}}, nil
	case "resources/read":
		return readResource(frame.Params)
	case "ping":
		return map[string]any{}, nil
	default:
		return nil, methodNotFound()
	}
}

func projectTool() map[string]any {
	return map[string]any{
		"name": "show_threadhall_project", "title": "Show Threadhall project brief",
		"description": "Show a compact interactive brief of the current Threadhall architecture.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{
			"focus": map[string]any{"type": "string", "description": "The collaboration topic to emphasize"},
		}},
		"annotations": map[string]any{
			"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false,
		},
		"_meta": map[string]any{
			"ui":             map[string]any{"resourceUri": ResourceURI, "visibility": []string{"model", "app"}},
			"ui/resourceUri": ResourceURI,
		},
	}
}

func callTool(raw json.RawMessage) (any, any) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if json.Unmarshal(raw, &params) != nil || params.Name != "show_threadhall_project" {
		return nil, methodNotFound()
	}
	focus, _ := params.Arguments["focus"].(string)
	if strings.TrimSpace(focus) == "" {
		focus = "human and agent collaboration"
	}
	data := map[string]any{
		"runtime": "Go", "storage": "SQLite", "focus": focus,
		"summary": "A lean, self-hosted team chat where humans and scoped agents work in the same channels.",
	}
	encoded, _ := json.Marshal(data)
	return map[string]any{
		"content":           []any{map[string]any{"type": "text", "text": string(encoded)}},
		"structuredContent": data, "isError": false,
	}, nil
}

func projectResource(withContent bool) map[string]any {
	resource := map[string]any{
		"uri": ResourceURI, "name": "Threadhall project brief", "description": "Interactive architecture summary", "mimeType": AppMIME,
	}
	if withContent {
		resource["text"] = appHTML
	}
	return resource
}

func readResource(raw json.RawMessage) (any, any) {
	var params struct {
		URI string `json:"uri"`
	}
	if json.Unmarshal(raw, &params) != nil || params.URI != ResourceURI {
		return nil, map[string]any{"code": -32002, "message": "resource not found"}
	}
	return map[string]any{"contents": []any{projectResource(true)}}, nil
}

func methodNotFound() any {
	return map[string]any{"code": -32601, "message": "method or tool not found"}
}
