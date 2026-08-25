# Task 7 report: deployable text-chat Preact client

## Design direction (recorded before implementation)

- **Visual thesis:** a calm, text-first workshop—warm paper conversation canvas, graphite navigation, rigorous spacing, one restrained vermilion thread accent; dense but breathable, no dashboard-card mosaic.
- **Content plan:** authentication/invite entry; left conversation navigation; primary message timeline/composer; right conversation/member context; responsive mobile drawers/sheets.
- **Interaction thesis:** restrained message entrance, clear reconnect/resync state transition, and fast mobile drawer/context transitions with reduced-motion support. Use CSS, no motion dependency.

## Files

- `web/src/api/{types,client}.ts`: stable public types and the single credentialed fetch path with CSRF, abort signals, and `ApiProblem` mapping.
- `web/src/auth/session.tsx`: session bootstrap, visible boot/authentication failures, sign-in, invite redemption, and sign-out.
- `web/src/realtime/socket.ts`: in-memory sequence cursor, event validation/deduplication, bounded exponential reconnect, and authoritative resync callback.
- `web/src/features/conversations/{list,create,detail}.tsx`: real conversation navigation, public/private/DM creation, and API-backed detail/member context.
- `web/src/features/messages/{timeline,composer}.tsx`: bounded chronological history, exact pending-key reconciliation, server-rendered HTML presentation, edit/delete, and keyboard-accessible composition.
- `web/src/layout/workspace.tsx`, `web/src/chat-workspace.tsx`, `web/src/app.tsx`: responsive three-pane shell and real API/socket orchestration.
- `web/src/styles.css`: paper/graphite/vermilion visual system, message/status/drawer motion, responsive panes, focus states, and reduced-motion behavior.
- Colocated `*.test.ts(x)` files plus `web/src/test/setup.ts`; Vite test setup; rebuilt embedded `internal/webassets/dist` files.

## TDD evidence

### RED

- `npm --prefix web test -- --run` failed as intended on 2026-08-26 before implementation: seven suites could not resolve the new `api/client`, `auth/session`, `conversations/list`, `messages/composer`, `messages/timeline`, `realtime/socket`, and `layout/workspace` modules. Exit code: 1.
- The failing tests cover login errors and invite redemption; exact channel/DM payloads and stable problems; channel selection; chronological event application; pending-send idempotency; server-rendered Markdown; edit/delete; reconnect/dedup/resync; keyboard composer behavior; and mobile drawer state.

### GREEN

- `npm --prefix web test -- --run`: 7 files, 13 tests passed.
- Coverage names/behaviors include login error and registration; exact channel/DM requests and stable problem errors; selection; chronological event insertion; pending-send idempotency; trusted server HTML; edit/delete; reconnect replay/dedup/resync; Enter vs Shift+Enter; visible send failure; and mobile drawer state.

## Verification and bundle

- `make check`: passed. This ran a clean `npm ci`, Vite production build, `go test -tags sqlite_fts5 ./...`, Vitest, a second embedded build, and `go build -tags sqlite_fts5`.
- Production bundle: JS 35.86 kB / 12.69 kB gzip; CSS 12.01 kB / 3.41 kB gzip; HTML 0.39 kB / 0.26 kB gzip. Initial compressed JS is well below 300 KiB.
- An initial plain `go test ./...` was intentionally superseded by the repository's required `sqlite_fts5` build tag after it reported `SQLite FTS5 support is required`; the canonical tagged full suite passed.
- Live embedded-binary smoke: bootstrapped a temporary admin and ran the built Mach-O binary against temporary real SQLite. Verified HTTP 200 for embedded index and hashed JS asset; admin login; invite creation/redemption into a separate member cookie session; public channel creation and membership; canonical DM identity across two creation keys; message send/edit/history; and server-rendered `<strong>` Markdown.
- Live two-session HTTP + WebSocket smoke: admin and member connected independently with their real session cookies, both received the same live `message.sent` at seq 6, the member disconnected, another HTTP message committed, and a fresh member socket using `after_seq=6` replayed seq 7 exactly. This exercised production HTTP/WS transport with two browser-equivalent cookie sessions; it was not a manual visual click-through in two graphical browser windows.

## Self-review

- No mock, demo, fallback, client Markdown parser, or speculative feature surface is present. Empty states remain empty and failures stay visible.
- All mutations use browser-generated idempotency keys. A pending send is keyed once, removed in `finally`, and reconciled only with the exact HTTP result; socket/entity merges cannot add a duplicate message.
- Socket state retains only the latest global sequence across reconnects. A `resync_required` envelope changes the visible state to `Resyncing`, performs HTTP list/detail/member/history reload, resets the cursor only after that callback, then reopens from zero.
- Fetch effects own and abort `AbortController`s on selection/session/socket cleanup. Collections are capped at 100 conversations, 100 members, 100-message pages, 200 retained timeline messages, and 20 pending sends.
- Every authored TS/TSX/CSS source is below the requested 300-line soft limit; the largest is 170 lines. `git diff --check` is clean.
- The frontend guidance shaped a cardless working surface: graphite navigation, warm paper timeline, a restrained vermilion accent, rigorous three-pane hierarchy, and CSS-only message/status/drawer transitions with reduced-motion support.

## Concerns

- `npm ci` emits engine warnings because this machine runs Node 23.11 while current Vitest/jsdom dependencies declare supported even-numbered Node releases; install, tests, and build still pass. CI should use a declared supported Node release.
- Conversation/member navigation intentionally shows the newest bounded 100-item API page; message history exposes earlier-page loading until the 200-message client cap. There is no unbounded client accumulation.
- DMs have no server-provided display name in the current contract, so navigation labels them by real conversation ID; the selected right pane shows the real member usernames.
- No threads, reactions, search, media, projects, work items, service worker, offline cache, UI/state/motion framework, or unsupported dependency was added.
