//go:build windows

package codex

import (
	"os/exec"
	"time"
)

func configureProcessCancellation(command *exec.Cmd) {
	command.WaitDelay = 2 * time.Second
}
