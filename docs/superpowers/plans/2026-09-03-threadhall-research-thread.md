# Threadhall Research Thread Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start a bounded Deep Research request in a named conversation thread with one action, while keeping all progress, failures, results, and artifacts in that thread.

**Architecture:** A small `research` service validates title and prompt, then one SQLite writer transaction creates a human-authored thread root, a plugin-bearing `@codex` invocation reply, a thread title, events, and the normal queued agent task. The worker path remains unchanged; the web form navigates to the returned root.

**Tech Stack:** Go, SQLite, existing agent task/plugin protocol, Preact, TypeScript, Vitest.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 7.

## Global Constraints

- Require the exact installed capability ID `deep-research-work@openai-curated-remote` on a live, explicitly granted `codex` agent before creating anything.
- Title is nonblank valid UTF-8 and at most 80 bytes. Research prompt is nonblank valid UTF-8 and at most 8 KiB.
- One request creates one top-level root plus one thread reply. Only the reply invokes the agent, so agent output and activity stay inside the thread.
- Creation is atomic and idempotent. No half-created root, client-side two-send sequence, silent retry, invented result, or widened membership/grant.

---

### Task 1: Define the research workflow contract

**Files:** Create `internal/research/model.go`, `internal/research/store.go`, `internal/research/service.go`, `internal/research/service_test.go`; modify `internal/agenttask/model.go`, `internal/agentd/codex/plugins.go`, and their tests.

- [ ] Add failing tests for invalid actor/conversation/title/prompt/key, 80-byte title and 8-KiB prompt limits, Markdown rendering, canonical invocation text, and clock propagation.
- [ ] Move the plugin ID to exported `agenttask.DeepResearchPluginID` and make default provisioning use that constant.
- [ ] Define `Start { ActorID, ConversationID int64; Title, Prompt, IdempotencyKey string }` and `Result { Root, Invocation message.Message; Events []realtime.Event }`.
- [ ] Render root and invocation Markdown server-side. Use exact invocation text `@codex [@Deep Research](plugin://deep-research-work@openai-curated-remote)` followed by a delimited “Research request” section containing the user's raw prompt as data.
- [ ] Define repository `Start(context.Context, Record) (Result, error)` and a distinct `ErrCapabilityUnavailable`.
- [ ] Run `go test ./internal/research ./internal/agenttask ./internal/agentd/codex`; expect success.
- [ ] Commit checkpoint `feat(research): define Deep Research thread workflow`.

### Task 2: Create the thread and task atomically

**Files:** Create `internal/store/sqlite/migrations/018_research_requests.sql`, `internal/store/sqlite/research_store.go`, `internal/store/sqlite/research_store_test.go`, `internal/store/sqlite/message_write.go`; modify `internal/store/sqlite/migrate.go`, `internal/store/sqlite/migrate_test.go`, `internal/store/sqlite/message_store.go`.

- [ ] Add failing tests for member success, outsider denial, absent/revoked/ungranted agent, absent capability, `human_only` policy, root/reply/thread-title shape, exactly one queued task, event order, complete rollback on injected failure, exact replay, and conflicting key reuse.
- [ ] Add `research_requests(actor_id, idempotency_key, fingerprint, conversation_id, root_message_id, invocation_message_id, created_at)` with a unique actor/key and foreign keys.
- [ ] Extract the current transaction-local message insert/event/task-queue logic into unexported helpers in `message_write.go`; preserve existing `MessageStore.Send` behavior and regression tests before using the helpers from research.
- [ ] In one writer transaction: replay-check, verify human membership and explicit agent policy, resolve the live granted `codex` agent with the exact plugin capability, insert root, insert invocation with `thread_root_id=root.id`, create `thread_titles`, queue one task, record research request, and commit both message events.
- [ ] Derive internal message idempotency keys as `research-root:` and `research-invoke:` plus the SHA-256 of actor ID and external key, keeping them below the existing 128-byte limit.
- [ ] Map unavailable capability to a stable non-retryable domain error; do not queue work if capability discovery is stale or absent.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/store/sqlite ./internal/message ./internal/research`; expect success.
- [ ] Commit checkpoint `feat(research): atomically start named research threads`.

### Task 3: Add one authenticated start endpoint

**Files:** Create `internal/httpapi/research.go`, `internal/httpapi/research_test.go`; modify `cmd/threadhall/main.go`.

- [ ] Add failing tests for session/CSRF, exact query/body allowlists, body size, authenticated actor injection, outsider 404, capability-unavailable 409 with code `capability_unavailable`, replay, no-store, and notification of the highest committed event sequence.
- [ ] Register `POST /api/v1/conversations/{conversation_id}/research` with `{title, prompt, idempotency_key}` and return 201 with root/invocation messages.
- [ ] Wire `research.Service` to `sqlite.ResearchStore`; reuse the same writer, clock, Markdown policy, and event pump.
- [ ] Ensure errors contain no agent token, plugin filesystem path, Codex home, or provider response.
- [ ] Run `go test ./internal/httpapi ./cmd/threadhall`; expect success.
- [ ] Commit checkpoint `feat(research): expose research thread start API`.

### Task 4: Add the research composer and thread navigation

**Files:** Modify `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/api/client.test.ts`, `web/src/chat-workspace.tsx`, `web/src/features/knowledge/panel.tsx`; create `web/src/features/research/start.tsx`, `start.test.tsx`; modify `web/src/styles.css`.

- [ ] Add failing tests for capability-present/absent states, title/prompt limits, submit disablement, visible API failure, duplicate-click suppression, returned-root navigation, and compact context drawer behavior.
- [ ] Add typed `startResearch` to `ApiClient`.
- [ ] Add “Start research” to the knowledge panel header only when the selected conversation exposes the exact Deep Research capability. If absent, keep a disabled control with a concise explanation rather than hiding the dependency.
- [ ] Submit one API mutation. On success, close the form, add the returned root to local timeline/thread summaries, open the named thread, and focus its invocation reply.
- [ ] Leave all subsequent progress, question cards, final output, plugin UI, and artifacts to the existing agent activity and `ThreadView` paths.
- [ ] Run `npm --prefix web test -- --run src/api/client.test.ts src/features/research src/chat-workspace.test.tsx`; expect success.
- [ ] Commit checkpoint `feat(web): start Deep Research in child threads`.

### Task 5: Verify failure and real-runtime boundaries

**Files:** Create `web/e2e/research.spec.ts`, `docs/verification/research-thread.md`; modify no production files unless tests reveal a defect.

- [ ] Add a fake-runtime browser test proving root/invocation creation, activity display in the thread, final artifact retention, question reply routing, pinning a finding, and promoting a conclusion to the parent conversation's decisions.
- [ ] Add failure cases for unavailable capability and worker failure; assert the former creates nothing and the latter remains a visible failed agent message in the thread without retry.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/research ./internal/store/sqlite ./internal/httpapi ./internal/agenttask`, `npm --prefix web run test:e2e -- research.spec.ts`, and `npm --prefix web run build`; expect success.
- [ ] If a configured live Codex worker is explicitly available, run one Deep Research smoke and record timestamp, plugin ID, task/thread IDs, and outcome under a clearly labeled “Live smoke” section. Otherwise record “not run” and why.
- [ ] Run `git diff --check` and commit `feat(research): ship scoped research threads`.
