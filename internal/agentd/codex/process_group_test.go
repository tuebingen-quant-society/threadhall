//go:build !windows

package codex

import (
	"bufio"
	"context"
	"os/exec"
	"testing"
	"time"
)

func TestConfigureProcessCancellationStopsChildHoldingStdout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 3 & wait")
	configureProcessCancellation(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = bufio.NewScanner(stdout).Scan()
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("child process kept Codex stdout open after cancellation")
	}
	_ = command.Wait()
}
