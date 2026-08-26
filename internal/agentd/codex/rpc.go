package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

const maxRPCFrameBytes = 1 << 20

var ErrInteractionRequired = errors.New("Codex requested an interactive question")

type Result struct {
	ThreadID  string
	Output    string
	Apps      []agenttask.InlineApp
	Questions []agenttask.Question
}

type rpcConnection struct {
	encoder *json.Encoder
	scanner *bufio.Scanner
	nextID  int64
}

type readWriter interface {
	io.Reader
	io.Writer
}

func runProtocol(ctx context.Context, transport readWriter, prompt, cwd string, config threadConfig) (Result, error) {
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(cwd) == "" {
		return Result{}, errors.New("Codex prompt and cwd are required")
	}
	scanner := bufio.NewScanner(transport)
	scanner.Buffer(make([]byte, 64<<10), maxRPCFrameBytes)
	rpc := &rpcConnection{encoder: json.NewEncoder(transport), scanner: scanner}
	if _, err := rpc.request(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{"name": "threadhall-agentd", "title": "Threadhall", "version": "0.1"},
	}); err != nil {
		return Result{}, fmt.Errorf("initialize Codex app server: %w", err)
	}
	if err := rpc.encoder.Encode(map[string]any{"method": "initialized"}); err != nil {
		return Result{}, fmt.Errorf("notify Codex initialization: %w", err)
	}
	threadRaw, err := rpc.request(ctx, "thread/start", threadStartParams(cwd, config))
	if err != nil {
		return Result{}, fmt.Errorf("start Codex thread: %w", err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(threadRaw, &thread); err != nil || thread.Thread.ID == "" {
		return Result{}, errors.New("Codex thread/start returned no thread id")
	}
	if _, err := rpc.request(ctx, "turn/start", map[string]any{
		"threadId": thread.Thread.ID,
		"input":    []any{map[string]any{"type": "text", "text": prompt}},
	}); err != nil {
		return Result{}, fmt.Errorf("start Codex turn: %w", err)
	}
	output, pendingApps, questions, err := rpc.readTurn(ctx)
	if err != nil {
		return Result{}, err
	}
	output, visualizations, err := extractVisualizations(output, cwd)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(output) == "" && len(questions) > 0 {
		output = questionOutput(questions)
	}
	if strings.TrimSpace(output) == "" {
		return Result{}, errors.New("Codex completed without a public response")
	}
	apps, err := rpc.readApps(ctx, thread.Thread.ID, pendingApps)
	if err != nil {
		return Result{}, err
	}
	apps = append(apps, visualizations...)
	if !agenttask.ValidInlineApps(apps) {
		return Result{}, errors.New("Codex returned invalid inline UI")
	}
	return Result{ThreadID: thread.Thread.ID, Output: output, Apps: apps, Questions: questions}, nil
}

func (r *rpcConnection) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	r.nextID++
	id := r.nextID
	if err := r.encoder.Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	for {
		frame, err := r.next(ctx)
		if err != nil {
			return nil, err
		}
		if frame.ID == id {
			if frame.Error != nil {
				return nil, fmt.Errorf("app server error %d: %s", frame.Error.Code, frame.Error.Message)
			}
			return frame.Result, nil
		}
		if frame.ID != 0 && frame.Method != "" {
			if frame.Method == "item/tool/requestUserInput" {
				_ = r.rejectServerRequest(frame.ID)
				return nil, ErrInteractionRequired
			}
			if err := r.rejectServerRequest(frame.ID); err != nil {
				return nil, err
			}
		}
	}
}

type pendingApp struct {
	Server, Tool, ResourceURI string
	Arguments, Result         json.RawMessage
}

func (r *rpcConnection) readTurn(ctx context.Context) (string, []pendingApp, []agenttask.Question, error) {
	var output strings.Builder
	apps := make([]pendingApp, 0, maxInlineApps)
	for {
		frame, err := r.next(ctx)
		if err != nil {
			return "", nil, nil, fmt.Errorf("read Codex turn: %w", err)
		}
		if frame.ID != 0 && frame.Method != "" {
			if frame.Method == "item/tool/requestUserInput" {
				questions, valid := captureQuestions(frame.Params)
				_ = r.rejectServerRequest(frame.ID)
				if !valid {
					return "", nil, nil, ErrInteractionRequired
				}
				return output.String(), apps, questions, nil
			}
			if err := r.rejectServerRequest(frame.ID); err != nil {
				return "", nil, nil, err
			}
			continue
		}
		switch frame.Method {
		case "item/completed":
			if len(apps) < maxInlineApps {
				if app, ok := completedApp(frame.Params); ok {
					apps = append(apps, app)
				}
			}
		case "item/agentMessage/delta":
			var params struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal(frame.Params, &params) == nil {
				if output.Len()+len(params.Delta) > agentOutputLimit {
					return "", nil, nil, errors.New("Codex response exceeds the public output limit")
				}
				output.WriteString(params.Delta)
			}
		case "turn/completed":
			var params struct {
				Turn struct {
					Status string `json:"status"`
				} `json:"turn"`
			}
			if err := json.Unmarshal(frame.Params, &params); err != nil {
				return "", nil, nil, errors.New("invalid turn/completed notification")
			}
			if params.Turn.Status != "completed" {
				return "", nil, nil, fmt.Errorf("Codex turn ended with status %q", params.Turn.Status)
			}
			return output.String(), apps, nil, nil
		}
	}
}

func completedApp(raw json.RawMessage) (pendingApp, bool) {
	var params struct {
		Item struct {
			Type, Server, Tool, Status string
			MCPAppResourceURI          string `json:"mcpAppResourceUri"`
			Arguments, Result          json.RawMessage
			AppContext                 *struct {
				ResourceURI string `json:"resourceUri"`
			} `json:"appContext"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &params) != nil || params.Item.Type != "mcpToolCall" || params.Item.Status != "completed" {
		return pendingApp{}, false
	}
	uri := params.Item.MCPAppResourceURI
	if params.Item.AppContext != nil && params.Item.AppContext.ResourceURI != "" {
		uri = params.Item.AppContext.ResourceURI
	}
	if params.Item.Server == "" || params.Item.Tool == "" || !strings.HasPrefix(uri, "ui://") ||
		len(params.Item.Arguments) > maxInlineAppJSON || len(params.Item.Result) > maxInlineAppJSON {
		return pendingApp{}, false
	}
	return pendingApp{Server: params.Item.Server, Tool: params.Item.Tool, ResourceURI: uri,
		Arguments: params.Item.Arguments, Result: params.Item.Result}, true
}

func (r *rpcConnection) readApps(ctx context.Context, threadID string, pending []pendingApp) ([]agenttask.InlineApp, error) {
	apps := make([]agenttask.InlineApp, 0, len(pending))
	total := 0
	for _, item := range pending {
		raw, err := r.request(ctx, "mcpServer/resource/read", map[string]any{
			"server": item.Server, "uri": item.ResourceURI, "threadId": threadID,
		})
		if err != nil {
			return nil, fmt.Errorf("read MCP App resource: %w", err)
		}
		var response struct {
			Contents []struct {
				URI, MIMEType, Text string
			} `json:"contents"`
		}
		if json.Unmarshal(raw, &response) != nil {
			return nil, errors.New("MCP App resource returned invalid content")
		}
		for _, content := range response.Contents {
			if content.URI != item.ResourceURI || !strings.EqualFold(strings.TrimSpace(content.MIMEType), mcpAppMIME) || content.Text == "" {
				continue
			}
			if len(content.Text) > maxInlineAppHTML || total+len(content.Text) > maxInlineAppsHTML {
				return nil, errors.New("MCP App resource exceeds the inline UI limit")
			}
			total += len(content.Text)
			apps = append(apps, agenttask.InlineApp{Server: item.Server, Tool: item.Tool,
				ResourceURI: item.ResourceURI, HTML: content.Text, Arguments: item.Arguments, Result: item.Result})
			break
		}
	}
	return apps, nil
}

const (
	agentOutputLimit  = 64 << 10
	maxInlineApps     = 4
	maxInlineAppJSON  = 64 << 10
	maxInlineAppHTML  = 256 << 10
	maxInlineAppsHTML = 512 << 10
	mcpAppMIME        = "text/html;profile=mcp-app"
)

type rpcFrame struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (r *rpcConnection) next(ctx context.Context) (rpcFrame, error) {
	select {
	case <-ctx.Done():
		return rpcFrame{}, ctx.Err()
	default:
	}
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return rpcFrame{}, err
		}
		return rpcFrame{}, io.EOF
	}
	var frame rpcFrame
	if err := json.Unmarshal(r.scanner.Bytes(), &frame); err != nil {
		return rpcFrame{}, fmt.Errorf("decode app server frame: %w", err)
	}
	return frame, nil
}

func (r *rpcConnection) rejectServerRequest(id int64) error {
	return r.encoder.Encode(map[string]any{
		"id": id, "error": map[string]any{"code": -32601, "message": "interactive controls are not enabled for this worker"},
	})
}
