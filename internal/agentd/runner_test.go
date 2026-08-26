package agentd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agentd/codex"
	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

func TestRunnerPublishesProgressThenCodexResult(t *testing.T) {
	t.Parallel()
	api := &fakeWorkerAPI{work: agenttask.Work{Task: agenttask.Task{ID: 7}, Prompt: "bounded"}, found: true}
	apps := []agenttask.InlineApp{{Server: "forms", Tool: "ask", ResourceURI: "ui://forms/ask", HTML: "<form></form>"}}
	questions := []agenttask.Question{{ID: "scope", Header: "Scope", Question: "Which scope?"}}
	runtime := &fakeRuntime{result: codex.Result{ThreadID: "runtime-id", Output: "## Answer", Apps: apps, Questions: questions}}
	runner := Runner{API: api, Runtime: runtime}
	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce = %v, %v", worked, err)
	}
	if runtime.prompt != "bounded" || api.progress != "Codex is working with the bounded conversation context…" {
		t.Fatalf("runtime prompt/progress = %q/%q", runtime.prompt, api.progress)
	}
	if api.output != "## Answer" || api.runtimeID != "runtime-id" || api.taskID != 7 {
		t.Fatalf("completion = task %d output %q runtime %q", api.taskID, api.output, api.runtimeID)
	}
	if len(api.apps) != 1 || api.apps[0].ResourceURI != "ui://forms/ask" {
		t.Fatalf("completion apps = %#v", api.apps)
	}
	if len(api.questions) != 1 || api.questions[0].ID != "scope" {
		t.Fatalf("completion questions = %#v", api.questions)
	}
}

func TestRunnerTimesOutAStuckCodexTurnAndPublishesFailure(t *testing.T) {
	t.Parallel()
	api := &fakeWorkerAPI{work: agenttask.Work{Task: agenttask.Task{ID: 10}, Prompt: "wait forever"}, found: true}
	runner := Runner{API: api, Runtime: blockingRuntime{}, TaskTimeout: 20 * time.Millisecond}
	started := time.Now()
	worked, err := runner.RunOnce(context.Background())
	if err != nil || !worked || time.Since(started) > time.Second {
		t.Fatalf("timed RunOnce = %v, %v after %s", worked, err, time.Since(started))
	}
	if !errors.Is(api.failure, context.DeadlineExceeded) || api.taskID != 10 {
		t.Fatalf("timeout failure = task %d, %v", api.taskID, api.failure)
	}
}

func TestRunnerPublishesSanitizedFailureAndKeepsWorkerAvailable(t *testing.T) {
	t.Parallel()
	api := &fakeWorkerAPI{work: agenttask.Work{Task: agenttask.Task{ID: 8}, Prompt: "ask a question"}, found: true}
	runtime := &fakeRuntime{err: codex.ErrInteractionRequired}
	worked, err := (Runner{API: api, Runtime: runtime}).RunOnce(context.Background())
	if err != nil || !worked {
		t.Fatalf("RunOnce failure = %v, %v", worked, err)
	}
	if !errors.Is(api.failure, codex.ErrInteractionRequired) || api.taskID != 8 {
		t.Fatalf("published failure = task %d, %v", api.taskID, api.failure)
	}
}

type fakeWorkerAPI struct {
	work              agenttask.Work
	found             bool
	progress          string
	output, runtimeID string
	taskID            int64
	failure           error
	apps              []agenttask.InlineApp
	questions         []agenttask.Question
}

func (f *fakeWorkerAPI) Next(context.Context) (agenttask.Work, bool, error) {
	return f.work, f.found, nil
}
func (f *fakeWorkerAPI) Progress(_ context.Context, _ int64, summary string) error {
	f.progress = summary
	return nil
}
func (f *fakeWorkerAPI) Complete(_ context.Context, taskID int64, output, runtimeID string, apps []agenttask.InlineApp, questions []agenttask.Question) error {
	f.taskID, f.output, f.runtimeID, f.apps, f.questions = taskID, output, runtimeID, apps, questions
	return nil
}
func (f *fakeWorkerAPI) Fail(_ context.Context, taskID int64, failure error) error {
	f.taskID, f.failure = taskID, failure
	return nil
}

type fakeRuntime struct {
	prompt string
	result codex.Result
	err    error
}

func (f *fakeRuntime) Run(_ context.Context, prompt string) (codex.Result, error) {
	f.prompt = prompt
	return f.result, f.err
}

type blockingRuntime struct{}

func (blockingRuntime) Run(ctx context.Context, _ string) (codex.Result, error) {
	<-ctx.Done()
	return codex.Result{}, ctx.Err()
}
