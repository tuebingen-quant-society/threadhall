package agentd

import (
	"context"
	"fmt"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agentd/codex"
	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

type WorkerAPI interface {
	Next(context.Context) (agenttask.Work, bool, error)
	Progress(context.Context, int64, string) error
	Complete(context.Context, int64, string, string, []agenttask.InlineApp, []agenttask.Question) error
	Fail(context.Context, int64, error) error
}

type Runtime interface {
	Run(context.Context, string) (codex.Result, error)
}

type Runner struct {
	API         WorkerAPI
	Runtime     Runtime
	PollWait    time.Duration
	TaskTimeout time.Duration
}

func (r Runner) RunOnce(ctx context.Context) (bool, error) {
	work, found, err := r.API.Next(ctx)
	if err != nil || !found {
		return false, err
	}
	if err := r.API.Progress(ctx, work.Task.ID, "Codex is working with the bounded conversation context…"); err != nil {
		return true, fmt.Errorf("publish agent progress: %w", err)
	}
	timeout := r.TaskTimeout
	if timeout <= 0 {
		timeout = 3 * time.Minute
	}
	taskContext, cancel := context.WithTimeout(ctx, timeout)
	result, err := r.Runtime.Run(taskContext, work.Prompt)
	cancel()
	if err != nil {
		if publishErr := r.API.Fail(ctx, work.Task.ID, err); publishErr != nil {
			return true, fmt.Errorf("publish Codex failure: %w", publishErr)
		}
		return true, nil
	}
	if err := r.API.Complete(ctx, work.Task.ID, result.Output, result.ThreadID, result.Apps, result.Questions); err != nil {
		return true, fmt.Errorf("publish Codex result: %w", err)
	}
	return true, nil
}

func (r Runner) Run(ctx context.Context) error {
	wait := r.PollWait
	if wait <= 0 {
		wait = time.Second
	}
	for {
		worked, err := r.RunOnce(ctx)
		if err != nil {
			return err
		}
		if worked {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
