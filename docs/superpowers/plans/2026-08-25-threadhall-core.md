# Threadhall Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver an invite-only single-workspace text chat with channels, DMs, ordered durable messages, reconnect replay, and a usable embedded web client.

**Architecture:** Domain packages expose storage interfaces; a bounded SQLite writer serializes mutations and commits a global event with each mutation. HTTP handles commands/history and a bounded WebSocket hub handles ordered events.

**Tech Stack:** Go, `net/http`, `database/sql`, `github.com/mattn/go-sqlite3`, `github.com/coder/websocket`, Preact, TypeScript, Vite, Vitest.

**Spec:** [`../specs/2026-08-25-threadhall-design.md`](../specs/2026-08-25-threadhall-design.md)

## Global Constraints

- Complete tasks in order and run the named failing test before implementation.
- Use temporary real SQLite databases for persistence tests; do not mock SQL behavior.
- Keep the writer input queue, socket send queue, frames, history pages, and event batches bounded.
- Return RFC 9457-style JSON problems with stable Threadhall codes; never expose internal errors.

---

### Task 1: Bootstrap the repository and contracts

**Files:** Create `go.mod`, `Makefile`, `.gitignore`, `LICENSE`, `README.md`, `cmd/threadhall/main.go`, `internal/app/app.go`, `internal/httpapi/problem.go`, `web/package.json`, `web/tsconfig.json`, `web/vite.config.ts`, `web/src/main.tsx`, `web/src/app.tsx`, `internal/webassets/assets.go`, `.github/workflows/ci.yml`.

- [ ] Write `internal/httpapi/problem_test.go` for JSON content type, status, stable `code`, and absence of an internal cause. Run `go test ./internal/httpapi`; expect failure because `Problem` does not exist.
- [ ] Implement the minimal public contract:

```go
type Problem struct { Status int `json:"status"`; Code string `json:"code"`; Detail string `json:"detail"` }
func WriteProblem(w http.ResponseWriter, p Problem)
```

- [ ] Add a `threadhall version` command, `/healthz`, embedded production assets, a minimal Preact shell, and Make targets `test`, `web`, `build`, and `check`.
- [ ] Run `go test ./...`, `npm --prefix web test -- --run`, and `npm --prefix web run build`; expect success and a runnable single binary.
- [ ] Commit with `chore: bootstrap Threadhall repository`.

### Task 2: Add configuration, SQLite startup, migrations, and bounded writes

**Files:** Create `internal/config/config.go`, `internal/config/config_test.go`, `internal/store/sqlite/db.go`, `writer.go`, `writer_test.go`, `migrate.go`, `migrate_test.go`, `migrations/001_core.sql`; modify `internal/app/app.go`.

- [ ] Test configuration validation for explicit state path, public URL, secure-cookie production rule, positive queue limits, and owner-only generated secret files. Run `go test ./internal/config`; expect failure.
- [ ] Test database pragmas (`foreign_keys=ON`, WAL, `synchronous=FULL`), refusal of a newer schema, and ordered writes with queue saturation returning `ErrBusy`. Run `go test ./internal/store/sqlite`; expect failure.
- [ ] Implement:

```go
type WriteFunc func(*sql.Tx) error
type Writer struct { requests chan request }
func (w *Writer) Do(ctx context.Context, fn WriteFunc) error
```

- [ ] Create initial tables for `users`, `sessions`, `invites`, `conversations`, `conversation_members`, `messages`, and `events`; embed and transactionally apply migrations.
- [ ] Run `go test -race ./internal/config ./internal/store/sqlite`; expect success with no leaked goroutines.
- [ ] Commit with `feat(store): add bounded SQLite persistence`.

### Task 3: Implement bootstrap, invites, passwords, and sessions

**Files:** Create `internal/auth/model.go`, `password.go`, `password_test.go`, `service.go`, `service_test.go`, `store.go`, `internal/store/sqlite/auth_store.go`, `auth_store_test.go`, `internal/httpapi/auth.go`, `auth_test.go`, `middleware.go`.

- [ ] Test Argon2id hash/verify, parameter encoding, malformed hashes, single-use expiring invite hashes, rotated hashed session tokens, and bootstrap refusal after the first admin. Run `go test ./internal/auth ./internal/store/sqlite`; expect failure.
- [ ] Define narrow storage operations and service commands:

```go
type CreateUser struct { Username, Password, InviteToken string }
type Store interface { Bootstrap(context.Context, Bootstrap) error; RedeemInvite(context.Context, CreateUser) (Session, error); RotateSession(context.Context, [32]byte) (Session, error) }
```

- [ ] Implement `/api/v1/session`, `/api/v1/invites`, `/api/v1/users`, session middleware, strict cookie attributes, origin validation, and double-submit CSRF for mutations.
- [ ] Add CLI `threadhall bootstrap-admin --username ...` reading the password from a TTY or stdin, never an argument.
- [ ] Run auth unit/integration tests plus `go test -race ./...`; expect success.
- [ ] Commit with `feat(auth): add invite-only authentication`.

### Task 4: Implement conversations and membership authorization

**Files:** Create `internal/conversation/model.go`, `store.go`, `service.go`, `service_test.go`, `internal/store/sqlite/conversation_store.go`, `conversation_store_test.go`, `internal/httpapi/conversations.go`, `conversations_test.go`; extend `migrations/001_core.sql` only if it has not shipped.

- [ ] Test public/private channel creation, exactly two distinct DM members, unique DM pairs, pagination bounds, and member/non-member reads. Run `go test ./internal/conversation ./internal/store/sqlite`; expect failure.
- [ ] Define `KindChannel`, `KindPrivate`, `KindDM` and require `CanRead(ctx, userID, conversationID)` at every history/event entry point.
- [ ] Implement create/list/detail/member HTTP endpoints with idempotency keys and stable conflict/forbidden codes.
- [ ] Add adversarial tests proving a valid user cannot enumerate or open an unjoined private channel or another DM.
- [ ] Run `go test -race ./internal/conversation ./internal/store/sqlite ./internal/httpapi`; expect success.
- [ ] Commit with `feat(conversations): add channels and direct messages`.

### Task 5: Implement durable text messages and history

**Files:** Create `internal/message/model.go`, `store.go`, `service.go`, `service_test.go`, `markdown.go`, `markdown_test.go`, `internal/store/sqlite/message_store.go`, `message_store_test.go`, `internal/httpapi/messages.go`, `messages_test.go`.

- [ ] Test bounded UTF-8 bodies, idempotent retry, stable IDs, authorization, keyset pagination, edit ownership, tombstone deletion, same-transaction `events.seq`, and server-side Markdown that strips scripts, event handlers, dangerous URLs, and raw HTML. Run relevant package tests; expect failure.
- [ ] Implement the command/result contract:

```go
type Send struct { ConversationID, AuthorID int64; Body, IdempotencyKey string }
type Result struct { Message Message; Event realtime.Event }
```

- [ ] Implement send/edit/delete/history endpoints; use integer cursor pagination, reject oversized bodies before entering the writer queue, and render Markdown through a fixed parser plus sanitizer policy.
- [ ] Verify direct SQL failure rolls back both domain row and event and duplicate idempotency returns the original result.
- [ ] Run `go test -race ./internal/message ./internal/store/sqlite ./internal/httpapi`; expect success.
- [ ] Commit with `feat(messages): add durable ordered text chat`.

### Task 6: Add bounded WebSocket fan-out and race-free replay

**Files:** Create `internal/realtime/event.go`, `hub.go`, `hub_test.go`, `replay.go`, `replay_test.go`, `socket.go`, `internal/httpapi/realtime.go`, `realtime_test.go`; modify app composition and message publication.

- [ ] Test socket authentication/origin, frame limit, event-count and byte-budget overflow, slow-client disconnect, authorization filtering, duplicate suppression, and the subscribe/high-water/replay race. Run `go test ./internal/realtime ./internal/httpapi`; expect failure.
- [ ] Implement `Hub.Subscribe(userID, afterSeq)`, capture the DB high-water mark after registration, replay authorized events through it, then forward only larger live sequences.
- [ ] Bound replay pages and return `resync_required` when `afterSeq` predates the retained minimum.
- [ ] Add a three-client integration test that pauses publication between subscription and replay and proves each authorized event arrives exactly once and in order.
- [ ] Run `go test -race ./internal/realtime ./internal/httpapi ./internal/store/sqlite`; expect success.
- [ ] Commit with `feat(realtime): add ordered event replay`.

### Task 7: Build the deployable text-chat PWA slice

**Files:** Create `web/src/api/client.ts`, `api/types.ts`, `realtime/socket.ts`, `auth/session.tsx`, `features/conversations/list.tsx`, `features/messages/timeline.tsx`, `features/messages/composer.tsx`, colocated `*.test.tsx`, `web/src/styles.css`; modify `web/src/app.tsx`.

- [ ] Write Vitest tests for login errors, channel selection, chronological event insertion, idempotent optimistic sends, safe Markdown presentation, edit/delete, reconnect replay, and keyboard-accessible composer. Run `npm --prefix web test -- --run`; expect failure.
- [ ] Implement one three-pane responsive shell using semantic controls and a single typed API/socket state path; show transport and validation errors visibly.
- [ ] Build assets and verify the Go production binary serves the app and never falls back to fabricated data.
- [ ] Run `npm --prefix web test -- --run`, `npm --prefix web run build`, and a manual two-browser text/reconnect smoke test.
- [ ] Commit with `feat(web): add realtime text chat client`.
