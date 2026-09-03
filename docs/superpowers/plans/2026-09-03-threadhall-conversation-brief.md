# Threadhall Conversation Brief Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every conversation one concise, versioned Markdown brief that members can edit safely and agents receive before transient chat history.

**Architecture:** The existing `knowledge` service gains optional brief reads and compare-and-swap updates. SQLite keeps one row per conversation and reuses knowledge mutation idempotency. The context pane adds read/edit modes, and agent task claims inject raw Markdown as their highest-priority user-authored section.

**Tech Stack:** Go, SQLite, Goldmark/BlueMonday through `message.RenderMarkdown`, Preact, TypeScript, Vitest.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 5.

## Global Constraints

- Store raw and server-sanitized rendered Markdown separately; never trust client HTML.
- One optional brief exists per conversation. Raw body is nonblank valid UTF-8 and at most 32 KiB.
- Create requires `expected_revision: 0`; each successful update increments revision by one. Replay the same idempotency key before checking revision.
- A stale write returns 409 with machine code `stale_revision` and the current revision; it never overwrites or auto-merges.
- Agents can read the current brief but cannot call its mutation route or directly update it from output.

---

### Task 1: Add the brief contract and conflict type

**Files:** Modify `internal/knowledge/model.go`, `internal/knowledge/store.go`, `internal/knowledge/service.go`; create `internal/knowledge/brief.go`, `internal/knowledge/brief_test.go`.

- [ ] Add failing tests for optional reads, 32-KiB validation, invalid UTF-8, blank Markdown, expected revision below zero, server rendering, idempotency validation, and repository clock/revision inputs.
- [ ] Define `Brief { ConversationID int64; Body, RenderedBody string; Revision int64; EditorID int64; Editor string; UpdatedAt time.Time }`, `BriefResult { Brief *Brief }`, `GetBrief`, and `PutBrief`.
- [ ] Define `StaleRevisionError { CurrentRevision int64 }` with `errors.Is(err, knowledge.ErrStaleRevision)` support.
- [ ] Call `message.RenderMarkdown` only after validating raw body; repository input carries both raw and rendered values.
- [ ] Run `go test ./internal/knowledge`; expect success.
- [ ] Commit checkpoint `feat(knowledge): define versioned briefs`.

### Task 2: Persist compare-and-swap briefs

**Files:** Create `internal/store/sqlite/migrations/016_conversation_briefs.sql`, `internal/store/sqlite/briefs.go`, `internal/store/sqlite/briefs_test.go`; modify `internal/store/sqlite/migrate.go`, `internal/store/sqlite/migrate_test.go`.

- [ ] Add failing tests for member create/update/read, outsider equivalence to missing, sequential revisions, stale conflict with current revision, exact idempotent replay despite a now-stale expected revision, key reuse conflict, editor attribution, and delete cascade.
- [ ] Add `conversation_briefs` keyed by `conversation_id` with body, rendered body, positive revision, editor, and updated timestamp.
- [ ] In one writer transaction: check prior mutation, verify membership, compare current revision, insert/update, emit `brief.updated`, and store the result in `knowledge_mutations`.
- [ ] Return `BriefResult{Brief:nil}` only after confirming current membership and no row. Never use absence to bypass authorization.
- [ ] Event payload contains revision, editor ID, and update time but not the body; clients refetch the authoritative brief.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/store/sqlite ./internal/knowledge`; expect success.
- [ ] Commit checkpoint `feat(knowledge): persist revisioned briefs`.

### Task 3: Expose conflict-aware HTTP and realtime behavior

**Files:** Create `internal/httpapi/briefs.go`, `internal/httpapi/briefs_test.go`; modify `internal/httpapi/knowledge.go`, `web/src/realtime/socket.ts`, `web/src/realtime/socket.test.ts`.

- [ ] Add failing HTTP tests for GET optional result, PUT session/CSRF, body limits, actor injection, no-store, generic outsider 404, stale 409 with `current_revision`, replay, and notifier behavior.
- [ ] Register `GET /api/v1/conversations/{conversation_id}/brief` and `PUT /api/v1/conversations/{conversation_id}/brief` with `{body, expected_revision, idempotency_key}`.
- [ ] Extend the problem writer only enough to serialize `current_revision` for `stale_revision`; keep all other problem shapes unchanged.
- [ ] Accept `brief.updated` in realtime validation and invalidate the brief only for its conversation.
- [ ] Run `go test ./internal/httpapi` and `npm --prefix web test -- --run src/realtime/socket.test.ts`; expect success.
- [ ] Commit checkpoint `feat(knowledge): expose versioned brief API`.

### Task 4: Add brief read/edit modes

**Files:** Modify `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/api/client.test.ts`, `web/src/features/knowledge/panel.tsx`, `web/src/chat-workspace.tsx`; create `web/src/features/knowledge/brief.tsx`, `brief.test.tsx`, `use-brief.ts`; modify `web/src/styles.css`.

- [ ] Add failing tests for empty/read/edit modes, rendered Markdown, 32-KiB field limit, save cancellation, stale conflict preserving the local draft, explicit Reload latest, retry with the new revision, selection aborts, and realtime invalidation.
- [ ] Add typed `getBrief` and `putBrief` client methods. Preserve `ApiProblem.current_revision` only when the server sends a valid nonnegative integer.
- [ ] Add a Brief tab before Pins in `KnowledgePanel`; load on conversation selection and keep mutation state in `useBrief`, not `ChatWorkspace`.
- [ ] On 409, show “The brief changed while you were editing” with Reload latest and Keep my draft; never silently retry or merge.
- [ ] Render `rendered_body` through the same sanitized message-body boundary and raw body only in the textarea.
- [ ] Run `npm --prefix web test -- --run src/api/client.test.ts src/features/knowledge src/chat-workspace.test.tsx`; expect success.
- [ ] Commit checkpoint `feat(web): add conversation brief editor`.

### Task 5: Put the brief first in agent context

**Files:** Modify `internal/agenttask/model.go`, `internal/agenttask/context.go`, `internal/agenttask/context_test.go`, `internal/store/sqlite/agent_store.go`, `internal/store/sqlite/agent_store_test.go`.

- [ ] Add failing tests for brief-before-pins ordering, raw Markdown, a 24-KiB brief context cap, no brief represented as `null`, outsider impossibility through grants, and history trimmed before brief/pins.
- [ ] Populate `ContextBundle.Brief` during the existing task-claim transaction. Truncate only on UTF-8 boundaries and label a truncated value explicitly.
- [ ] Reserve at most 24 KiB for the brief within the existing 128-KiB total context budget.
- [ ] Keep provider output unable to call knowledge stores; proposals remain ordinary replies/artifacts.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/agenttask ./internal/store/sqlite ./internal/agentd`; expect success.
- [ ] Run `git diff --check` and commit `feat(agents): ground tasks in conversation briefs`.
