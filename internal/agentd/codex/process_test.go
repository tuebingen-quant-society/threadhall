package codex

import (
	"context"
	"testing"
)

func TestClientRejectsRelativeWorkingDirectoryBeforeStartingCodex(t *testing.T) {
	t.Parallel()
	client := Client{Command: "codex", Cwd: "relative/path"}
	if _, err := client.Run(context.Background(), "bounded prompt"); err == nil {
		t.Fatal("Run relative cwd error = nil, want rejection")
	}
}
