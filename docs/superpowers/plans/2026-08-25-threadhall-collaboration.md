# Threadhall Collaboration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the deployable text-chat slice with one-level threads, explicit channel forks, reactions, unread state, authorized FTS5 search, streaming media, and resilient responsive PWA behavior.

**Architecture:** Collaboration features remain mutations through the bounded SQLite writer. Media bytes stream to content-addressed local storage while SQLite holds authorization metadata. The web client derives UI state from authoritative HTTP data plus ordered events.

**Tech Stack:** Go, SQLite FTS5, local filesystem, Preact, TypeScript, Vitest, Playwright.

**Spec:** [`../specs/2026-08-25-threadhall-design.md`](../specs/2026-08-25-threadhall-design.md)

## Global Constraints

- Threads are one-level child streams, effectively message-scoped subchannels: every reply
  references the root, replies cannot become roots, and channels do not nest recursively.
- Forks are real new conversations with independent membership and agent grants. They retain an
  authorization-checked source edge rather than duplicating conversation data.
- Search, attachments, unreads, and events must apply membership authorization in storage queries.
- Stream uploads without buffering full files; use sniffed MIME types and generated paths only.
- Do not add transcoding, unfurls, offline writes, read receipts, or custom emoji.

---

### Task 1: Add threads, channel forks, and reactions

**Files:** Modify `internal/message/model.go`, `internal/conversation/model.go`, their stores/services and HTTP handlers; create/extend colocated tests; create `web/src/features/threads/panel.tsx`, `features/conversations/fork.tsx`, `features/reactions/bar.tsx`, and tests; add `migrations/004_threads_forks_reactions.sql`.

- [ ] Add failing tests for root replies, rejection of nested replies/cross-conversation roots, fork creation from a root or thread, independent fork membership/grants, authorized source backlinks, unique user/emoji reactions, toggling, tombstoned roots, and unauthorized thread/fork reads.
- [ ] Extend message commands with `ThreadRootID *int64` and define `Reaction { MessageID, UserID int64; Emoji string }`; allow only a small server-owned Unicode emoji set in v0.1.
- [ ] Define `ConversationFork { ConversationID, SourceConversationID, SourceRootMessageID int64 }`; create the target conversation and source edge atomically without copying message bodies.
- [ ] Implement transactional reply/fork/reaction mutations and durable events, then HTTP endpoints and accessible UI controls.
- [ ] Run `go test -race ./internal/message ./internal/store/sqlite ./internal/httpapi` and `npm --prefix web test -- --run`; expect success.
- [ ] Commit with `feat(messages): add threads and reactions`.

### Task 2: Add unread cursors and authorized search

**Files:** Create `internal/message/search.go`, `search_test.go`, `internal/store/sqlite/search_store.go`, `search_store_test.go`, `internal/httpapi/search.go`, `search_test.go`; modify conversation membership/store; create `web/src/features/search/search.tsx`, `features/conversations/unread.ts`, and tests; add `migrations/005_search.sql`.

- [ ] Add failing tests for monotonic per-conversation read cursors, bounded unread counts, FTS query/page limits, edits/deletes reflected in FTS, snippets escaped as data, and exclusion of inaccessible conversations.
- [ ] Define:

```go
type SearchQuery struct { UserID int64; Text string; BeforeID int64; Limit int }
type SearchResult struct { MessageID, ConversationID int64; Snippet string }
```

- [ ] Implement FTS5 maintenance in the same write transaction as message changes and membership-filtered search joins.
- [ ] Add mark-read and search endpoints, unread badges, and an accessible search results surface.
- [ ] Run focused Go/web tests plus `go test -race ./...`; expect success.
- [ ] Commit with `feat(search): add unread cursors and authorized FTS`.

### Task 3: Implement streaming content-addressed uploads

**Files:** Create `internal/media/model.go`, `store.go`, `service.go`, `service_test.go`, `filesystem.go`, `filesystem_test.go`, `internal/store/sqlite/media_store.go`, `media_store_test.go`, `internal/httpapi/media.go`, `media_test.go`; add `migrations/006_media.sql`.

- [ ] Add failing tests for hard byte limit, MIME sniffing, SHA-256 deduplication, owner-only temp mode, atomic rename, quota reservation, pending token expiry, message claim, delete denial, and traversal filenames.
- [ ] Implement the streaming boundary:

```go
type Upload struct { OwnerID int64; Filename string; Body io.Reader; DeclaredSize int64 }
type Stored struct { Token string; Hash [32]byte; Size int64; MIME string }
```

- [ ] Reserve quota before accepting declared bytes, reconcile while streaming, `fsync` and rename by digest, and store display filename separately from paths.
- [ ] Serve media only after session and message-membership checks, with `nosniff`, safe disposition, range requests, and no directory exposure.
- [ ] Run media tests under `go test -race`, plus fuzz tests for filenames and content-disposition generation.
- [ ] Commit with `feat(media): add bounded local file storage`.

### Task 4: Claim attachments and enforce lifecycle cleanup

**Files:** Modify message commands/stores and media service; create `internal/media/cleanup.go`, `cleanup_test.go`; modify app startup/shutdown and HTTP message DTOs.

- [ ] Add failing integration tests proving a pending token is owner-bound, single-claim, atomically attached with its message, immediately inaccessible after deletion, and physically removed after grace.
- [ ] Implement attachment claims inside the message transaction and a single-instance bounded cleanup loop for 24-hour pending files and seven-day deleted files.
- [ ] Add quota-conflict and cleanup metrics to structured logs without filenames, bodies, IPs, or tokens.
- [ ] Run `go test -race ./internal/media ./internal/message ./internal/store/sqlite ./internal/httpapi`; expect success.
- [ ] Commit with `feat(media): attach files and enforce retention`.

### Task 5: Add media, thread, and reconnect UI flows

**Files:** Create `internal/realtime/ephemeral.go`, `ephemeral_test.go`, `web/src/features/media/uploader.tsx`, `media/attachment.tsx`, `media/progress.ts`, `web/src/realtime/ephemeral.ts`, extend thread/reaction/search views and tests; create `web/e2e/collaboration.spec.ts`, `web/playwright.config.ts`; modify `web/package.json`.

- [ ] Write failing Go/component tests for upload progress/error/cancel, safe filenames, image/file presentation, expired attachment, thread focus, reaction state, unread cursor, replay deduplication, `resync_required` reload, and rate-limited memory-only typing/presence with TTL expiry.
- [ ] Implement bounded client upload and timeline state; render accepted images/video through native browser elements and all other media as authenticated downloads. Add non-durable typing/presence events without database writes.
- [ ] Write Playwright flows for two users across desktop and mobile widths: channel, thread, reaction, image, disconnect/reconnect, and authorized/unauthorized search.
- [ ] Run component tests, production web build, and `npm --prefix web run test:e2e`; expect success.
- [ ] Commit with `feat(web): complete collaboration flows`.

### Task 6: Make the collaboration slice an installable PWA

**Files:** Create `web/public/manifest.webmanifest`, `web/src/pwa/register.ts`, icons, `web/e2e/pwa.spec.ts`; modify Vite config, HTML metadata, CSS, app shell, and Go cache headers.

- [ ] Add failing tests for keyboard navigation, focus restoration, labels, reduced motion, responsive drawer behavior, manifest validity, update notification, and no offline mutation queue.
- [ ] Cache only versioned shell assets; keep API/media network-authoritative and show explicit offline/reconnect state.
- [ ] Run Playwright desktop/mobile accessibility smoke, Lighthouse PWA audit, and compressed-bundle measurement; record results in `docs/verification/collaboration.md`.
- [ ] Commit with `feat(web): ship responsive installable PWA`.
