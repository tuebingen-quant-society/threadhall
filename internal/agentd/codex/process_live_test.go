package codex

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLiveAuthenticatedCodexAppServer(t *testing.T) {
	if os.Getenv("THREADHALL_LIVE_CODEX") != "1" {
		t.Skip("set THREADHALL_LIVE_CODEX=1 to use the authenticated local Codex runtime")
	}
	command, err := exec.LookPath("codex")
	if err != nil {
		t.Fatalf("find codex: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := (Client{Command: command, Cwd: t.TempDir()}).Run(ctx,
		"Reply with the exact marker THREADHALL_CODEX_LIVE_OK and one short Markdown bullet. Do not use tools.")
	if err != nil {
		t.Fatalf("live Codex run: %v", err)
	}
	if !strings.Contains(result.Output, "THREADHALL_CODEX_LIVE_OK") || result.ThreadID == "" {
		t.Fatalf("live Codex result = %#v", result)
	}
}
