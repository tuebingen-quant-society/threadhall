# Threadhall Pins Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let members preserve important messages and their attached artifacts as reversible conversation pins, then include a bounded projection in agent context.

**Architecture:** A new `knowledge` domain owns pin commands and reads. SQLite stores references to live messages and emits conversation events atomically. The context pane renders joined message data, while agent task claims receive raw pin text through a sectioned context bundle.

**Tech Stack:** Go, SQLite, Preact, TypeScript, Vitest.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 4 and Shared Context Contract.

## Global Constraints

- A pin references one message in the same conversation; artifact data remains only on that message.
- All current members may pin/unpin. A missing/inaccessible/cross-conversation message returns the same generic not-found response.
- List at most 50 pins newest first. Agent context uses at most 20 pins and 24 KiB of raw text.
- Editing a message changes its pin projection. Soft-deleting a message removes its pin in the same transaction and does not retain its text elsewhere.

---

### Task 1: Define the knowledge/pin domain

**Files:** Create `internal/knowledge/errors.go`, `internal/knowledge/model.go`, `internal/knowledge/store.go`, `internal/knowledge/service.go`, `internal/knowledge/service_test.go`.

- [ ] Add failing service tests for invalid IDs, blank/oversized idempotency keys, default/bounded list limits, and clock propagation.
- [ ] Define `Pin { ConversationID, MessageID, ActorID int64; Actor string; Message message.Message; CreatedAt time.Time }`, `PinPage`, `CreatePin`, `RemovePin`, and `ListPins`.
- [ ] Define repository methods `CreatePin`, `RemovePin`, and `ListPins`, returning committed `realtime.Event` values for mutations.
- [ ] Reuse `message.ValidIdempotencyKey`; reject limits above 50 and set a default of 50.
- [ ] Run `go test ./internal/knowledge`; expect success.
- [ ] Commit checkpoint `feat(knowledge): define conversation pins`.

### Task 2: Persist pins and deletion semantics

**Files:** Create `internal/store/sqlite/migrations/015_pins.sql`, `internal/store/sqlite/knowledge_store.go`, `internal/store/sqlite/pins.go`, `internal/store/sqlite/pins_test.go`; modify `internal/store/sqlite/migrate.go`, `internal/store/sqlite/migrate_test.go`, `internal/store/sqlite/message_edits.go`, `internal/store/sqlite/message_store_test.go`.

- [ ] Add failing store tests for member create/remove, duplicate replay, conflicting idempotency-key reuse, outsider denial, cross-conversation denial, newest-first listing, message edits reflected, and message deletion removing the pin.
- [ ] Add `conversation_pins(conversation_id, message_id, actor_id, created_at)` with a composite primary key and indexed newest-first order. Add reusable `knowledge_mutations(actor_id, idempotency_key, operation, fingerprint, result_json, created_at)` for this and later knowledge writes.
- [ ] Implement create as `INSERT ... SELECT` joined through `conversation_members` and a live same-conversation message. Treat already-pinned with a new key as success returning the existing projection.
- [ ] Implement explicit remove plus `pin.removed`; implement create plus `pin.created`. Event payloads contain only message ID, actor ID, and timestamp.
- [ ] In `MessageStore.Delete`, detect and delete an existing pin before tombstoning the message, emitting `pin.removed` and `message.deleted` in the same writer transaction. Return the message event as today; the pump discovers both committed sequences.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/store/sqlite ./internal/knowledge ./internal/message`; expect success.
- [ ] Commit checkpoint `feat(knowledge): persist message pins`.

### Task 3: Add the authenticated pin API and realtime projection

**Files:** Create `internal/httpapi/knowledge.go`, `internal/httpapi/pins.go`, `internal/httpapi/pins_test.go`; modify `cmd/threadhall/main.go`, `web/src/realtime/socket.ts`, `web/src/realtime/socket.test.ts`.

- [ ] Add failing HTTP tests for session/CSRF enforcement, query allowlists, actor injection, same not-found response for inaccessible sources, idempotent replay, `no-store`, and notifier calls.
- [ ] Register `GET /api/v1/conversations/{conversation_id}/pins`, `POST /api/v1/conversations/{conversation_id}/pins`, and `DELETE /api/v1/conversations/{conversation_id}/pins/{message_id}`; mutation bodies carry `idempotency_key`, and create also carries `message_id`.
- [ ] Wire one `knowledge.Service` and `sqlite.KnowledgeStore` in `newServerHandler`.
- [ ] Extend realtime validation for `pin.created` and `pin.removed` with exact bounded payload shapes. Make either event invalidate only the selected conversation's pin list.
- [ ] Run `go test ./internal/httpapi ./cmd/threadhall` and `npm --prefix web test -- --run src/realtime/socket.test.ts`; expect success.
- [ ] Commit checkpoint `feat(knowledge): expose realtime pin API`.

### Task 4: Build the pin UI in a reusable knowledge panel

**Files:** Modify `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/api/client.test.ts`, `web/src/chat-workspace.tsx`, `web/src/features/messages/message-row.tsx`, `web/src/features/threads/view.tsx`; create `web/src/features/knowledge/panel.tsx`, `pin-list.tsx`, `use-pins.ts`, and colocated tests; modify `web/src/styles.css`.

- [ ] Add failing component tests for loading/empty/error states, pin/unpin from channel and thread rows, duplicate click suppression, actor/time display, artifact-bearing pin display, source navigation/focus, and mobile context drawer behavior.
- [ ] Add typed `listPins`, `pinMessage`, and `unpinMessage` client methods.
- [ ] Replace the default `ConversationDetail` context content with a `KnowledgePanel` that retains Overview/Members and adds Pins. Keep brief/decision tabs absent until their stages.
- [ ] Add a Pin/Unpin message action driven by the selected conversation's pin-ID set. Split message actions into a separate component if `message-row.tsx` would exceed 300 LOC.
- [ ] On pin selection, open the existing conversation/thread and focus the source message using the exact-context navigation built by search.
- [ ] Run `npm --prefix web test -- --run src/api/client.test.ts src/features/knowledge src/features/messages src/features/threads src/chat-workspace.test.tsx`; expect success.
- [ ] Commit checkpoint `feat(web): add conversation pin workflow`.

### Task 5: Add bounded pins to agent context

**Files:** Modify `internal/agenttask/model.go`, `internal/agenttask/context.go`, `internal/agenttask/context_test.go`, `internal/store/sqlite/agent_store.go`, `internal/store/sqlite/agent_store_test.go`.

- [ ] Add failing tests for ordering, 20-item/24-KiB pin limits, raw text rather than rendered HTML, deleted pins absent, thread invocation receiving conversation pins, and recent history trimmed before pin text when the global prompt budget is reached.
- [ ] Introduce `ContextBundle { Brief *ContextBrief; Decisions []ContextDecision; Pins []ContextPin; Messages []ContextMessage }`; keep brief/decisions empty in this slice.
- [ ] Change `Work.Context` to `ContextBundle`. Query pins in the same read transaction as task claim, joined through the task's already-validated conversation grant.
- [ ] Serialize labeled JSON sections inside `<threadhall_context>` in the approved order and retain the explicit statement that every section is untrusted data.
- [ ] Keep a 128-KiB total context ceiling: reserve accepted higher-priority sections first and retain the newest complete history messages in the remaining bytes.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/agenttask ./internal/store/sqlite ./internal/agentd`; expect success.
- [ ] Run `git diff --check` and commit `feat(agents): include bounded conversation pins`.
