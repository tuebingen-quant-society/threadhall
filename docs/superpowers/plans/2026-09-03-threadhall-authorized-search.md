# Threadhall Authorized Search Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a signed-in member find current message text across only their conversations and open the exact matching message or thread.

**Architecture:** A standalone `search` service validates a simple bounded term query. SQLite FTS5 triggers maintain the index in each message transaction; reads join indexed rows back through live messages and membership. A target-context endpoint returns a bounded authorized window so UI navigation never pages blindly.

**Tech Stack:** Go, SQLite FTS5, Preact, TypeScript, Vitest, Playwright.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 3.

## Global Constraints

- Search current non-deleted raw message text only. Briefs, decisions, rendered HTML, activity labels, filenames, and plugin results are excluded.
- Accept 1–8 whitespace-separated terms, at most 256 UTF-8 bytes total. Quote each normalized term for FTS; do not expose raw FTS syntax.
- Return at most 25 results, each snippet at most 240 UTF-8 bytes, ordered by descending message ID with `before_id` keyset pagination.
- An absent conversation, absent message, deleted message, and inaccessible message all return the same generic not-found response.

---

### Task 1: Define and validate the search contract

**Files:** Create `internal/search/model.go`, `internal/search/store.go`, `internal/search/service.go`, `internal/search/service_test.go`.

- [ ] Add failing tests for blank/invalid UTF-8/oversized queries, more than eight terms, negative cursors, limit 26, quote escaping, default limit 25, and stable normalized terms.
- [ ] Define `Query { UserID, BeforeID int64; Text string; Limit int }`, `Hit { MessageID, ConversationID, AuthorID int64; ThreadRootID *int64; Author, Snippet string; CreatedAt time.Time }`, `Page`, and `Context { Root *message.Message; Messages []message.Message; TargetID int64 }`.
- [ ] Define repository methods `Search(context.Context, Query) (Page, error)` and `Context(context.Context, userID, conversationID, messageID int64) (Context, error)`.
- [ ] Implement validation and term quoting without accepting `OR`, column selectors, prefix operators, or caller-provided quotes as operators.
- [ ] Run `go test ./internal/search`; expect success.
- [ ] Commit checkpoint `feat(search): define authorized search contract`.

### Task 2: Add transactional FTS storage

**Files:** Create `internal/store/sqlite/migrations/014_message_search.sql`, `internal/store/sqlite/search_store.go`, `internal/store/sqlite/search_store_test.go`; modify `internal/store/sqlite/migrate.go`, `internal/store/sqlite/migrate_test.go`.

- [ ] Add failing migration/store tests for backfilling existing live messages, indexing a send, replacing text on edit, removing text on delete, excluding an inaccessible private conversation and DM, snippets without stored HTML, keyset paging, and exact context for both channel and thread hits.
- [ ] Create external-content FTS5 table `message_search(body, content='messages', content_rowid='id', tokenize='unicode61')` plus insert/update/delete triggers. Update/delete triggers must issue the FTS delete command before optional reinsertion.
- [ ] Implement `SearchStore` with `MATCH ?`, live-message and `conversation_members` joins, `m.id < ?` when paginating, `ORDER BY m.id DESC`, and `limit+1` cursor detection.
- [ ] Use SQLite `snippet` markers only as plain sentinel bytes; replace them with a server-owned plain-text excerpt and truncate on UTF-8 boundaries before returning.
- [ ] Implement `Context` as the target plus at most 24 earlier items in the same top-level or thread scope; include the thread root separately when the target is a reply.
- [ ] Run `go test -tags sqlite_fts5 ./internal/store/sqlite ./internal/search`; expect success.
- [ ] Commit checkpoint `feat(search): add transactional FTS index`.

### Task 3: Expose authenticated HTTP reads

**Files:** Create `internal/httpapi/search.go`, `internal/httpapi/search_test.go`; modify `cmd/threadhall/main.go`.

- [ ] Add failing handler tests for session requirement, query-key allowlist, target length cap, default/bounded limits, authenticated user injection, generic not-found context, `Cache-Control: no-store`, and successful JSON shapes.
- [ ] Register `GET /api/v1/search/messages?q=...&before_id=...&limit=...` and `GET /api/v1/conversations/{conversation_id}/search-context/{message_id}`.
- [ ] Reuse the existing session middleware and problem document shape; map validation to 400, inaccessible/missing to 404, and SQLite busy to 503.
- [ ] Wire one `search.Service` and `sqlite.SearchStore` in `newServerHandler`.
- [ ] Run `go test ./internal/httpapi ./cmd/threadhall`; expect success.
- [ ] Commit checkpoint `feat(search): expose scoped message search`.

### Task 4: Build exact-result navigation

**Files:** Modify `web/src/api/types.ts`, `web/src/api/client.ts`, `web/src/api/client.test.ts`, `web/src/chat-workspace.tsx`; create `web/src/features/search/panel.tsx`, `panel.test.tsx`, `navigation.ts`, `navigation.test.ts`; modify `web/src/styles.css`.

- [ ] Add failing client/component tests for debounced query, cancellation, pagination, empty/error states, escaped snippet rendering, keyboard result selection, and no request below one normalized term.
- [ ] Add `searchMessages` and `searchContext` client methods plus exact TypeScript response types.
- [ ] Add an accessible search button and dialog/panel in the navigation column. Keep its query/results state outside `useWorkspace`.
- [ ] Implement a navigation coordinator: select the hit conversation, install the returned bounded context into a new `useWorkspace.showSearchContext`, open `ThreadView` with the returned root when needed, then focus `#message-{targetID}` after render.
- [ ] Treat a realtime edit/delete after opening a hit as authoritative; merge the event normally and close the preview only if its target disappears.
- [ ] Run `npm --prefix web test -- --run src/api/client.test.ts src/features/search src/chat-workspace.test.tsx`; expect success.
- [ ] Commit checkpoint `feat(web): add exact message search navigation`.

### Task 5: Verify authorization and compact behavior

**Files:** Create `web/e2e/search.spec.ts`; update no production files unless the test reveals a defect.

- [ ] Add a browser flow with two members and one outsider across a public channel, private channel, DM, and named thread; assert the outsider cannot infer private text, author, count, or context URL.
- [ ] Assert edited terms replace old hits, deleted messages vanish, a thread hit opens its thread and receives focus, and a top-level old hit opens without loading every newer page.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/search ./internal/store/sqlite ./internal/httpapi`, `npm --prefix web run test:e2e -- search.spec.ts`, `npm --prefix web run typecheck`, and `npm --prefix web run build`; expect success.
- [ ] Run `git diff --check` and commit `feat(search): ship authorized message search`.
