# Threadhall Agent Collaboration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explicitly invoked, scoped agent teammates through an outbound worker, with Codex task execution, isolated worktrees, visible progress, exact approvals, artifacts, and native structured question cards.

**Architecture:** Threadhall persists identity, grants, task state, decisions, and chat delivery. `threadhall-agentd` keeps provider/Git secrets and connects outbound using a versioned protocol. A Codex adapter normalizes app-server items without exposing provider objects to chat.

**Tech Stack:** Go, WebSocket, SQLite, Codex app-server JSON-RPC, Git worktrees, Preact/TypeScript.

**Spec:** [`../specs/2026-08-25-threadhall-agent-design.md`](../specs/2026-08-25-threadhall-agent-design.md)

## Global Constraints

- Passive messages never invoke agents; context access is bounded, authorized, and audited.
- Browser/server never receive provider keys, Git credentials, or worker filesystem paths.
- Raw reasoning, secrets, full command output, and provider errors never become chat messages.
- Only exact approve-once or deny decisions exist; provider output cannot grant authority.
- Support Codex only in v0.1 while preserving provider-neutral task/event types.

---

### Task 1: Define and contract-test the worker protocol

**Files:** Create `internal/agentprotocol/version.go`, `message.go`, `task.go`, `interaction.go`, `codec.go`, `codec_test.go`, `testdata/*.json`.

- [ ] Write failing golden tests for version negotiation, authenticate, start, steer, interrupt, resume, answer, progress, approval, question, artifact, failure, usage, heartbeat, and unknown message rejection.
- [ ] Define bounded envelopes with task/session/interaction IDs and monotonic per-connection sequence:

```go
type Envelope struct { Version uint16; Seq uint64; Type Type; Payload json.RawMessage }
const MaxEnvelopeBytes = 256 << 10
```

- [ ] Implement strict JSON decoding with unknown-field rejection, field/collection limits, and redacted `String` methods.
- [ ] Fuzz decoder framing and run `go test -race ./internal/agentprotocol`; expect success.
- [ ] Commit with `feat(agent): define worker protocol`.

### Task 2: Add agent identities, grants, connection authentication, and invocation

**Files:** Create `internal/agenttask/model.go`, `store.go`, `service.go`, `service_test.go`, `internal/store/sqlite/agent_store.go`, `agent_store_test.go`, `internal/httpapi/agents.go`, `agents_test.go`, `agent_socket.go`, `agent_socket_test.go`; add `migrations/007_agents.sql`.

- [ ] Add failing tests for hashed revocable worker tokens, identity/channel/repository grants, explicit mention/DM/action invocation, one active task per thread, owner follow-up, non-owner re-mention, and cross-channel denial.
- [ ] Implement admin endpoints for identities/grants/token rotation and an origin-independent worker WebSocket authenticated by token scope over TLS.
- [ ] Persist a task and visible task-root event atomically before sending `start`; leave tasks queued/interrupted when no worker is available.
- [ ] Add adversarial tests proving worker and human identities cannot fetch context outside stored grants.
- [ ] Run focused tests and `go test -race ./...`; expect success.
- [ ] Commit with `feat(agent): add scoped identities and tasks`.

### Task 3: Build `threadhall-agentd` lifecycle and Codex adapter

**Files:** Create `cmd/threadhall-agentd/main.go`, `internal/agentd/config.go`, `client.go`, `runner.go`, `runner_test.go`, `internal/agentd/codex/process.go`, `rpc.go`, `adapter.go`, `adapter_test.go`, `testdata/*.json`.

- [ ] Add failing tests for outbound auth, heartbeat, bounded task concurrency, reconnect, child process cancellation, app-server `thread/start`, `thread/resume`, `turn/start`, `turn/steer`, and `turn/interrupt` mapping.
- [ ] Define a provider seam limited to the approved lifecycle:

```go
type Runtime interface { Start(context.Context, Start) (Session, <-chan Event, error); Steer(context.Context, Session, string) error; Interrupt(context.Context, Session) error; Answer(context.Context, Session, Answer) error }
```

- [ ] Supervise Codex as a process group, bound stdout/stderr frames, redact public failures, persist no credentials, and make shutdown interrupt exact active turns.
- [ ] Verify adapter golden fixtures against the installed Codex app-server schema and run race tests.
- [ ] Commit with `feat(agentd): add Codex app-server runtime`.

### Task 4: Add bounded context, progress, delivery, stop, and recovery

**Files:** Create `internal/agenttask/context.go`, `context_test.go`, `delivery.go`, `recovery.go`, tests; create `internal/agentd/context.go`, `progress.go`, tests; modify worker socket and web task views.

- [ ] Add failing tests for invocation-first context, bounded thread/attachment fetch, membership recheck/audit, progress coalescing, stop ownership, final-before-progress-removal, disconnect interruption, and explicit resume without silent restart.
- [ ] Implement authenticated worker context endpoints with byte/item/time budgets and delimited untrusted content.
- [ ] Coalesce high-frequency activity to at most one update per task per second; persist only lifecycle transitions and sanitized summaries.
- [ ] Add native task status/progress/stop UI and reconnect recovery states.
- [ ] Run Go/web tests plus a fake-runtime disconnect/resume integration flow; expect success.
- [ ] Commit with `feat(agent): add visible task lifecycle`.

### Task 5: Isolate registered repositories in Git worktrees

**Files:** Create `internal/agentd/repository/registry.go`, `registry_test.go`, `worktree.go`, `worktree_test.go`, `path.go`, `path_test.go`; extend agentd config and task start mapping.

- [ ] Add failing tests for admin-registered canonical roots, repository grants, safe branch names, symlink/path escape, one worktree per coding thread, cleanup rules, dirty-base handling, and concurrent tasks.
- [ ] Create worktrees under an agentd-owned root using argument-vector Git execution; never shell-concatenate task input.
- [ ] Start Codex with workspace-write and approval-on-request in the exact worktree; refuse unknown or ungranted repositories.
- [ ] Fuzz repository/reference path parsing and run tests with a real temporary Git repository.
- [ ] Commit with `feat(agentd): isolate repository worktrees`.

### Task 6: Implement exact approval gates and artifacts

**Files:** Create `internal/agenttask/approval.go`, `approval_test.go`, `artifact.go`, `artifact_test.go`, SQLite store additions, HTTP handlers; create `internal/agentd/approval.go`, `artifact.go`, tests; create web approval/artifact components and tests; add `migrations/008_agent_controls.sql`.

- [ ] Add failing tests for admin-only decision, sanitized action summary, canonical digest, expiry, atomic claim, replay/stale mismatch, approve-once, deny, and provider-forged decision rejection.
- [ ] Gate network, push, PR, merge, destructive command, and external-message operation classes; resume only the exact stored digest.
- [ ] Represent artifacts as worker-owned metadata plus authenticated expiring retrieval; never expose worker paths.
- [ ] Render approval and artifact cards with exact scope, expiry, decision identity, keyboard controls, and visible failure.
- [ ] Run adversarial Go/web tests and commit with `feat(agent): add approvals and artifacts`.

### Task 7: Implement durable structured question cards

**Files:** Create `internal/agenttask/question.go`, `question_test.go`, SQLite interaction store/HTTP handlers; create `internal/agentd/codex/question.go`, `question_test.go`; create `web/src/features/agents/question-card.tsx`, tests; extend migration 008 only if unshipped.

- [ ] Add failing golden tests normalizing Codex `item/tool/requestUserInput` IDs, at most three questions, single/multi/confirm/other/free-text schemas, deadline, and secret-request rejection.
- [ ] Validate owner/admin authorization and the stored schema, atomically claim one answer, return `409` for stale/duplicate answers, expire at provider deadline or 24 hours, and resume the exact runtime turn.
- [ ] Render a server-owned accessible card and immutable answer summary; reject agent HTML/CSS/script/component descriptions.
- [ ] Run component tests and a real Codex smoke where the agent asks, waits, receives an answer, resumes, and finishes.
- [ ] Commit with `feat(agent): add interactive question cards`.

### Task 8: Verify the complete agent acceptance story

**Files:** Create `internal/agenttask/acceptance_test.go`, `web/e2e/agent.spec.ts`, `docs/verification/agents.md`; add sanitized protocol fixtures as needed.

- [ ] Exercise two humans and one remote agent through invoke, bounded context, worktree, progress, steering, question, approval denial/approval, artifact, stop, disconnect, resume, and final delivery.
- [ ] Prove a member cannot decide approvals, a different user cannot answer the question, another channel cannot access task context, and provider text cannot forge control events.
- [ ] Run `go test -race ./...`, frontend tests, Playwright agent flows, and a manual real-Codex run; record exact versions/results.
- [ ] Commit with `test(agent): cover end-to-end collaboration`.
