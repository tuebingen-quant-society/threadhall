// Command threadhall-agentd runs an outbound Threadhall Codex worker.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agentd"
	"github.com/tuebingen-quant-society/threadhall/internal/agentd/codex"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, arguments []string, getenv func(string) string) error {
	flags := flag.NewFlagSet("threadhall-agentd", flag.ContinueOnError)
	threadhallURL := flags.String("threadhall-url", "", "Threadhall HTTPS URL or loopback HTTP URL")
	codexCommand := flags.String("codex-command", "codex", "Codex executable")
	codexCwd := flags.String("codex-cwd", "", "absolute read-only working directory for chat tasks")
	once := flags.Bool("once", false, "process at most one queued task")
	taskTimeout := flags.Duration("task-timeout", 3*time.Minute, "maximum duration of one Codex turn")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	token := getenv("THREADHALL_WORKER_TOKEN")
	client, err := agentd.NewClient(*threadhallURL, token, &http.Client{Timeout: 35 * time.Second})
	if err != nil {
		return fmt.Errorf("configure Threadhall worker: %w", err)
	}
	if *taskTimeout < time.Minute || *taskTimeout > 30*time.Minute {
		return errors.New("task-timeout must be between one and thirty minutes")
	}
	runner := agentd.Runner{
		API: client, Runtime: codex.Client{Command: *codexCommand, Cwd: *codexCwd},
		PollWait: time.Second, TaskTimeout: *taskTimeout,
	}
	if *once {
		worked, err := runner.RunOnce(ctx)
		if err != nil {
			return err
		}
		if !worked {
			return errors.New("no queued task")
		}
		return nil
	}
	return runner.Run(ctx)
}
