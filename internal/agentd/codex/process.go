package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxPublicStderrBytes = 8 << 10

// Client supervises one authenticated local Codex App Server per task.
type Client struct {
	Command                 string
	Cwd                     string
	Model                   string
	ReasoningEffort         string
	SubagentModel           string
	SubagentReasoningEffort string
	MaxConcurrentSubagents  int
}

func (c Client) Run(ctx context.Context, prompt string) (Result, error) {
	if strings.TrimSpace(prompt) == "" {
		return Result{}, errors.New("absolute Codex cwd, command, and prompt are required")
	}
	taskCwd, err := os.MkdirTemp(c.Cwd, ".threadhall-task-")
	if err != nil {
		return Result{}, fmt.Errorf("create isolated Codex task directory: %w", err)
	}
	defer os.RemoveAll(taskCwd)
	command, stdin, stdout, err := c.start(ctx)
	if err != nil {
		return Result{}, err
	}
	result, protocolErr := runProtocol(ctx, struct {
		io.Reader
		io.Writer
	}{Reader: stdout, Writer: stdin}, prompt, taskCwd, threadConfig{
		Model: c.Model, ReasoningEffort: c.ReasoningEffort,
		SubagentModel: c.SubagentModel, SubagentReasoningEffort: c.SubagentReasoningEffort,
		MaxConcurrentSubagents: c.MaxConcurrentSubagents,
	})
	stopProcess(command, stdin)
	if protocolErr != nil {
		return Result{}, fmt.Errorf("Codex app server protocol: %w", protocolErr)
	}
	return result, nil
}

func (c Client) start(ctx context.Context) (*exec.Cmd, io.WriteCloser, io.ReadCloser, error) {
	if c.Command == "" || !filepath.IsAbs(c.Cwd) {
		return nil, nil, nil, errors.New("absolute Codex cwd and command are required")
	}
	info, err := os.Stat(c.Cwd)
	if err != nil || !info.IsDir() {
		return nil, nil, nil, errors.New("Codex cwd must be an existing directory")
	}
	command := exec.CommandContext(ctx, c.Command, "app-server")
	configureProcessCancellation(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open Codex stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open Codex stdout: %w", err)
	}
	stderr := &boundedBuffer{remaining: maxPublicStderrBytes}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("start Codex app server: %w", err)
	}
	return command, stdin, stdout, nil
}

func stopProcess(command *exec.Cmd, stdin io.Closer) {
	_ = stdin.Close()
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	_ = command.Wait()
}

type boundedBuffer struct {
	buffer    bytes.Buffer
	remaining int
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	original := len(value)
	if len(value) > b.remaining {
		value = value[:b.remaining]
	}
	if len(value) > 0 {
		_, _ = b.buffer.Write(value)
		b.remaining -= len(value)
	}
	return original, nil
}
