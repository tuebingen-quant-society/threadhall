# Threadhall Decisions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve attributable, immutable decisions that members can promote from discussion and explicitly supersede as the team changes course.

**Architecture:** Decisions are append-only knowledge rows. Superseding creates a new row and atomically links the old row to it; normal APIs never edit/delete statements. Optional message sources remain authorization-checked references, while copied decision text remains stable.

**Tech Stack:** Go, SQLite, Preact, TypeScript, Vitest.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 6.

## Global Constraints

- Decision statements are nonblank valid UTF-8, normalized only with outer whitespace trimming, and at most 4 KiB.
- Members create and supersede; there is no normal update/delete endpoint.
- An optional source message must be live and in the same conversation at creation. Copy the submitted statement; later source edits do not alter it.
- List at most 100 decisions newest first. Agent context contains at most 20 active decisions and 16 KiB; superseded decisions never enter normal prompts.

---

### Task 1: Define append-only decisions

**Files:** Modify `internal/knowledge/model.go`, `internal/knowledge/store.go`, `internal/knowledge/service.go`; create `internal/knowledge/decisions.go`, `internal/knowledge/decisions_test.go`.

- [ ] Add failing tests for statement/ID/source validation, 4-KiB and UTF-8 boundaries, default/bounded list limits, create, supersede, and idempotency clock propagation.
- [ ] Define `Decision { ID, ConversationID, ActorID int64; Statement, Actor, Status string; SourceMessageID, SupersededByID *int64; CreatedAt time.Time }`, `DecisionPage`, `CreateDecision`, `SupersedeDecision`, and `ListDecisions`.
- [ ] Restrict status output to `active` or `superseded`; status is repository-derived, never accepted from a client.
- [ ] Define repository create/supersede methods returning both the decision and committed event.
- [ ] Run `go test ./internal/knowledge`; expect success.
- [ ] Commit checkpoint `feat(knowledge): define attributable decisions`.

### Task 2: Persist immutable decision chains

**Files:** Create `internal/store/sqlite/migrations/017_decisions.sql`, `internal/store/sqlite/decisions.go`, `internal/store/sqlite/decisions_test.go`; modify `internal/store/sqlite/migrate.go`, `internal/store/sqlite/migrate_test.go`.

- [ ] Add failing tests for member create/list, outsider denial, cross-conversation/deleted source denial, copied statement stability after source edit, active/superseded projections, only-active supersession, concurrent supersession conflict, replay, key conflict, and conversation cascade.
- [ ] Add `conversation_decisions` with immutable statement/actor/source/created columns and nullable self-reference `superseded_by_id`; add a partial unique index preventing two rows from superseding the same active decision.
- [ ] Create by verifying membership and optional live same-conversation source in the insert query, then emit `decision.created` and record the mutation.
- [ ] Supersede in one transaction: authenticate membership, require the old row's `superseded_by_id IS NULL`, insert the new row, update the old link, emit `decision.superseded`, and record replay data.
- [ ] On reads, left join source messages only when not deleted; retain the source ID but omit its preview after deletion.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/store/sqlite ./internal/knowledge`; expect success.
- [ ] Commit checkpoint `feat(knowledge): persist append-only decisions`.

### Task 3: Expose decision reads and mutations

**Files:** Create `internal/httpapi/decisions.go`, `internal/httpapi/decisions_test.go`; modify `internal/httpapi/knowledge.go`, `web/src/realtime/socket.ts`, `web/src/realtime/socket.test.ts`.

- [ ] Add failing HTTP tests for bounded list parsing, session/CSRF, actor injection, source validation errors, conflict mapping, no-store, generic outsider 404, replay, and notifier sequences.
- [ ] Register `GET /api/v1/conversations/{conversation_id}/decisions`, `POST /api/v1/conversations/{conversation_id}/decisions`, and `POST /api/v1/conversations/{conversation_id}/decisions/{decision_id}/supersessions`.
- [ ] Use `{statement, source_message_id?, idempotency_key}` for both mutation bodies; the path identifies the old decision during supersession.
- [ ] Add exact realtime validators for `decision.created` and `decision.superseded`; event bodies carry IDs, actor, status/link, and timestamp but not the statement.
- [ ] Run `go test ./internal/httpapi` and `npm --prefix web test -- --run src/realtime/socket.test.ts`; expect success.
- [ ] Commit checkpoint `feat(knowledge): expose decision API`.

### Task 4: Add promotion and decision history UI

**Files:** Modify `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/api/client.test.ts`, `web/src/features/knowledge/panel.tsx`, `web/src/features/messages/message-row.tsx`, `web/src/features/threads/view.tsx`; create `web/src/features/knowledge/decisions.tsx`, `decision-form.tsx`, `use-decisions.ts`, and colocated tests; modify `web/src/styles.css`.

- [ ] Add failing tests for active/superseded sections, actor/time/source display, promote-from-message prefilling, explicit confirmation, manual decision creation, supersede form, concurrent conflict, source navigation, and deleted-source behavior.
- [ ] Add typed list/create/supersede client methods and immutable decision types.
- [ ] Add Decisions after Brief and before Pins in `KnowledgePanel`; default to active and provide a separate collapsed superseded history.
- [ ] Add “Record decision” to channel/thread message actions. Open a form with the source body prefilled but editable; submit only after the human confirms the final statement.
- [ ] Add Supersede only to active decisions; show both old and replacement after success. Do not add Edit or Delete controls.
- [ ] Run `npm --prefix web test -- --run src/api/client.test.ts src/features/knowledge src/features/messages src/features/threads src/chat-workspace.test.tsx`; expect success.
- [ ] Commit checkpoint `feat(web): add decision promotion workflow`.

### Task 5: Add active decisions to agent context

**Files:** Modify `internal/agenttask/model.go`, `internal/agenttask/context.go`, `internal/agenttask/context_test.go`, `internal/store/sqlite/agent_store.go`, `internal/store/sqlite/agent_store_test.go`.

- [ ] Add failing tests for brief → active decisions → pins → recent history ordering, 20-item/16-KiB limits, attribution included, superseded rows excluded, and newest active decisions retained when bounded.
- [ ] Populate `ContextBundle.Decisions` in the claim transaction with only rows whose `superseded_by_id IS NULL`.
- [ ] Serialize ID, actor, timestamp, and statement as untrusted JSON data; do not include rendered HTML or source-message bodies.
- [ ] Reserve decision bytes after the brief and before pins; trim history first, then pins, while never exceeding the 128-KiB total.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/agenttask ./internal/store/sqlite ./internal/agentd`; expect success.
- [ ] Run `git diff --check` and commit `feat(agents): include active decisions in context`.
