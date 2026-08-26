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
	Command string
	Cwd     string
}

func (c Client) Run(ctx context.Context, prompt string) (Result, error) {
	if c.Command == "" || !filepath.IsAbs(c.Cwd) || strings.TrimSpace(prompt) == "" {
		return Result{}, errors.New("absolute Codex cwd, command, and prompt are required")
	}
	info, err := os.Stat(c.Cwd)
	if err != nil || !info.IsDir() {
		return Result{}, errors.New("Codex cwd must be an existing directory")
	}
	command := exec.CommandContext(ctx, c.Command, "app-server")
	stdin, err := command.StdinPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open Codex stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Result{}, fmt.Errorf("open Codex stdout: %w", err)
	}
	stderr := &boundedBuffer{remaining: maxPublicStderrBytes}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return Result{}, fmt.Errorf("start Codex app server: %w", err)
	}
	transport := struct {
		io.Reader
		io.Writer
	}{Reader: stdout, Writer: stdin}
	result, protocolErr := runProtocol(ctx, transport, prompt, c.Cwd)
	_ = stdin.Close()
	if command.Process != nil {
		_ = command.Process.Kill()
	}
	_ = command.Wait()
	if protocolErr != nil {
		return Result{}, fmt.Errorf("Codex app server protocol: %w", protocolErr)
	}
	return result, nil
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
