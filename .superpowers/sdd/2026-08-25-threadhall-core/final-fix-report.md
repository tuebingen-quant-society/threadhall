# Threadhall Core final remediation report

Date: 2026-08-26

Baseline: `507a70cd51ee092845e4258f619856535a1e6544`

## Outcome and bounded decisions

- Legacy message idempotency uses transactional lazy adoption. Shipped migration
  `003_messages.sql` remains byte-for-byte unchanged, while v2 upgrades and existing
  v3 databases with unledgered rows are both covered. An exact root send adopts its
  original durable `message.sent` result; mismatched sends and edit/delete reuse reject
  before side effects.
- The timeline retains at most 200 messages and 200 unresolved per-entity patches.
  Initial/authoritative history selects the latest window; an explicit older-page load
  slides to the older window. A subsequent realtime or HTTP entity is pinned visible
  while preserving the exposed older edge. HTTP/entity sequence state remains separate
  from the socket reconnect cursor.
- Conversation invalidation retains exactly one dirty unit across failures. Non-abort
  failures retry from 500 ms with capped 15 s exponential backoff; replay bursts coalesce,
  success clears stale errors and resets backoff, and stop/unmount clears retry timers.
- Strict TypeScript now recognizes Vite CSS imports and the native WebSocket constructor.
  `npm run typecheck` is part of `make web`, and therefore the canonical `make check`/CI
  path, without weakening `strict`.
- Future migrations are contiguous after Core `001`-`003`: collaboration owns
  `004_threads_reactions`, `005_search`, and `006_media`; agents owns `007_agents` and
  `008_agent_controls` (question cards may extend unshipped `008`). No later feature was
  implemented.

## TDD evidence

### Legacy message upgrade

RED:

`go test -tags sqlite_fts5 ./internal/store/sqlite -run TestMessageStoreAdoptsOnlyExactVersionTwoSendIdempotency -count=1`

- exact legacy retry returned `message.ErrConflict` instead of message 7/event 11;
- edit reuse returned nil and changed the message;
- delete reuse returned nil and tombstoned the message.

GREEN:

`go test -tags sqlite_fts5 ./internal/store/sqlite -run 'TestMessageStoreAdoptsOnlyExactVersionTwoSendIdempotency|TestMessageStore(Sends|Edits|Scopes|Rolls)' -count=1`

The focused SQLite package passed. The fixture applies real shipped migrations 001 and
002, seeds the legacy message/event, closes it, and upgrades through `Open` to v3.

### Delayed entities and the 200-message window

RED:

`npm test -- --run src/features/messages/timeline.test.tsx src/chat-workspace-message-window.test.tsx`

Six regressions failed: bounded patches were absent; edit and delete each lost to delayed
initial history and delayed older history; and the third page beyond 200 advanced its
cursor while message 2 remained invisible.

GREEN:

`npm test -- --run src/features/messages/timeline.test.tsx src/chat-workspace-message-window.test.tsx src/chat-workspace.test.tsx src/realtime/socket.test.ts`

Four files and 28 tests passed. The window regression asserts cursor requests
`undefined -> 201 -> 101`, exactly 200 unique rendered entities, the newly exposed older
page, deduplication, and then-new realtime message 301 while message 2 remains visible.

### Failed invalidation retry

RED:

`npm test -- --run src/features/chat/invalidation.test.ts` failed three retry/timer cases:
no retry after 500 ms, no retained capped timer under a 10,000-request burst, and no timer
to clean on stop. The component RED then showed retry success left the visible
`reload unavailable` error behind.

GREEN:

`npm test -- --run src/chat-workspace-invalidation-retry.test.tsx src/features/chat/invalidation.test.ts src/chat-workspace-coordination.test.tsx src/chat-workspace.test.tsx`

Four files and 26 tests passed, covering fail-retry-success, capped/coalesced backoff,
timer cleanup after unmount, stale-error clearing, selection preservation, and existing
list/selection generation races.

### TypeScript gate

RED: `./node_modules/.bin/tsc --noEmit` failed with TS2882 for `styles.css`, TS2322 for
the native WebSocket constructor, plus strict TS2304/TS2769 findings in the newly authored
timeline regression/reducer.

GREEN: `npm run typecheck` passed with `strict: true` and no emitted files.

## Final verification

- `gofmt` on all changed Go files: completed.
- `git diff --check`: passed.
- `go test -race -tags sqlite_fts5 ./...`: passed across all packages.
- `go vet -tags sqlite_fts5 ./...`: passed with no output.
- `npm test -- --run`: 14 files, 60 tests passed.
- `npm run typecheck`: passed.
- `npm run build`: passed; 24 modules transformed.
- `make check`: passed after `npm ci`; it ran typecheck, production build, tagged Go
  tests, all 60 frontend tests, and the tagged Go binary build.

Production bundle: HTML 0.39 kB / 0.26 kB gzip; CSS 12.33 kB / 3.47 kB gzip;
JavaScript 47.82 kB / 16.02 kB gzip. Initial compressed JS remains below 300 KiB.

## Scope and caveats

- No accepted deferred minor was changed except future plan migration numbering.
- No authored file exceeds 300 lines. The largest changed production file is
  `web/src/features/chat/use-workspace.ts` at 287 lines; timeline is 229 lines.
- Local `make check` under Node 23.11.0 emitted dependency engine warnings. Install,
  typecheck, tests, build, and audit still passed; CI already declares Node 24.x.
- No blocker remains. Nothing was pushed.

## Exceptional Wave: residual timeline races

Baseline: `ab1e11241167bc5d44bc6a07a22a2c6c78a166ea`

### Decisions

- Unresolved patch overflow now advances a monotonic history generation. Initial and
  older requests capture that generation and may neither merge entities nor advance
  their HTTP cursor after overflow. Overflow coalesces an authoritative latest-page
  recovery through the existing bounded retry primitive; a successful recovery clears
  superseded patch ordering state. Repeated overflow retains one in-flight/queued unit,
  uses capped backoff if its own generation becomes stale, and stops its timer on
  socket cleanup.
- Timeline retention has one policy for realtime, mutation-result, initial-history,
  and older-history merges. It reserves a bounded 20-entity recently changed segment
  and fills the remaining 180 slots from the active latest or older edge. Thus an
  exposed older page and recent realtime entities coexist, dedupe remains by message
  ID, and the total stays at 200. Pins are compacted with messages/unresolved patches,
  so the extra ordering state is bounded.

### TDD evidence

RED:

`npm test -- --run src/chat-workspace-message-window.test.tsx`

Five of ten tests failed before the production change. At 201 distinct unloaded
entities, edit and delete each failed to trigger authoritative recovery for a delayed
initial response and for a delayed older response. In the full-window race, realtime
message 601 disappeared when the delayed third older page merged.

GREEN:

`npm test -- --run src/chat-workspace-message-window.test.tsx`

All ten tests passed. The regressions cover edit and delete overflow against literal
delayed initial and older responses, exactly one recovery request for the 201-event
burst, rejection of the stale response, usable subsequent pagination, a delayed full
older window with realtime arriving first, cursor progression
`undefined -> 501 -> 401 -> 301`, deduplication, and exactly 200 visible entities.

### Exceptional Wave verification

- `git diff --check`: passed.
- `npm test -- --run`: 14 files, 65 tests passed.
- `npm run typecheck`: passed.
- `npm run build`: passed; 24 modules transformed.
- `go test -race -tags sqlite_fts5 ./...`: passed across all packages.
- `go vet -tags sqlite_fts5 ./...`: passed with no output.
- `make check`: passed after `npm ci`; it ran strict typecheck, production build,
  tagged Go tests, all 65 frontend tests, and the tagged Go binary build.

Exceptional Wave production bundle: HTML 0.39 kB / 0.26 kB gzip; CSS 12.33 kB /
3.47 kB gzip; JavaScript 48.98 kB / 16.38 kB gzip. No authored production file
exceeds the 300-line soft limit: `use-workspace.ts` is 300 lines and `timeline.tsx`
is 250 lines. No deferred feature or plan was touched, and nothing was pushed.
