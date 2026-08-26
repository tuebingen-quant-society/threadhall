package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const maxRPCFrameBytes = 1 << 20

var ErrInteractionRequired = errors.New("Codex requested an interactive question")

type Result struct {
	ThreadID string
	Output   string
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

func runProtocol(ctx context.Context, transport readWriter, prompt, cwd string) (Result, error) {
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
	threadRaw, err := rpc.request(ctx, "thread/start", map[string]any{
		"cwd": cwd, "approvalPolicy": "never", "sandbox": "read-only", "ephemeral": true,
		"developerInstructions": "Answer as a Threadhall teammate. Do not inspect the filesystem, external services, or other Codex threads. Use only the supplied bounded chat context.",
	})
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
	output, err := rpc.readTurn(ctx)
	if err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(output) == "" {
		return Result{}, errors.New("Codex completed without a public response")
	}
	return Result{ThreadID: thread.Thread.ID, Output: output}, nil
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

func (r *rpcConnection) readTurn(ctx context.Context) (string, error) {
	var output strings.Builder
	for {
		frame, err := r.next(ctx)
		if err != nil {
			return "", fmt.Errorf("read Codex turn: %w", err)
		}
		if frame.ID != 0 && frame.Method != "" {
			if frame.Method == "item/tool/requestUserInput" {
				_ = r.rejectServerRequest(frame.ID)
				return "", ErrInteractionRequired
			}
			if err := r.rejectServerRequest(frame.ID); err != nil {
				return "", err
			}
			continue
		}
		switch frame.Method {
		case "item/agentMessage/delta":
			var params struct {
				Delta string `json:"delta"`
			}
			if json.Unmarshal(frame.Params, &params) == nil {
				if output.Len()+len(params.Delta) > agentOutputLimit {
					return "", errors.New("Codex response exceeds the public output limit")
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
				return "", errors.New("invalid turn/completed notification")
			}
			if params.Turn.Status != "completed" {
				return "", fmt.Errorf("Codex turn ended with status %q", params.Turn.Status)
			}
			return output.String(), nil
		}
	}
}

const agentOutputLimit = 64 << 10

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
