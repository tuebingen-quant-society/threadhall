# Codex teammate verification

Verified locally on 2026-08-26 against `codex-cli 0.146.1`, authenticated with
ChatGPT login, using the Codex App Server stdio protocol. The worker starts one
fresh ephemeral Codex thread per Threadhall task with `approvalPolicy=never`,
`sandbox=read-only`, and an empty absolute working directory.

## Passed

- A live App Server schema-compatible smoke returned the expected marker in
  35.4 seconds without an API key.
- An explicit `@codex` mention in a granted channel created one durable task,
  showed progress, and replaced that same teammate message with the final
  Markdown response.
- The bounded context contained the invoking channel and recovered the in-scope
  canary value `42`. A canary stored in another private conversation was absent
  from the prompt and response.
- A mention in that ungranted private conversation created no task and no agent
  message.
- Flipping a conversation from `explicit` to `human_only` changed an already
  queued task to `denied`; restarting the worker did not execute it.
- A mention in a one-level discussion thread received only the thread root and
  replies, and the Codex response remained inside that thread.
- A second authenticated human session saw the same agent-authored result and
  rendered Markdown.

## Gaps found

- GitHub-style Markdown is sanitized and rendered, including headings, lists,
  inline code, and fenced blocks. Token-level syntax highlighting is not yet
  implemented; fenced code is currently a styled plain code block.
- Native `requestUserInput` question cards are not implemented. The live test
  exposed a stuck progress state; the worker now has a bounded task timeout and
  unsupported interactions can be finalized as a visible sanitized failure.
- File/artifact replies are not implemented, so no authenticated upload and
  download E2E claim is made.
- Approvals, runtime resume after worker loss, repository worktrees, and durable
  per-step task cards remain in the approved agent roadmap.

## Reproducible checks

```sh
THREADHALL_LIVE_CODEX=1 go test -tags sqlite_fts5 \
  ./internal/agentd/codex -run TestLiveAuthenticatedCodexAppServer -count=1 -v
go test -race -tags sqlite_fts5 ./...
npm --prefix web test -- --run
go vet -tags sqlite_fts5 ./...
```
