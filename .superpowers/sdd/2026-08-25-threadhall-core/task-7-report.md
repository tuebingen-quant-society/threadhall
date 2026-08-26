# Task 7 report: deployable text-chat Preact client

## Design direction (recorded before implementation)

- **Visual thesis:** a calm, text-first workshop—warm paper conversation canvas, graphite navigation, rigorous spacing, one restrained vermilion thread accent; dense but breathable, no dashboard-card mosaic.
- **Content plan:** authentication/invite entry; left conversation navigation; primary message timeline/composer; right conversation/member context; responsive mobile drawers/sheets.
- **Interaction thesis:** restrained message entrance, clear reconnect/resync state transition, and fast mobile drawer/context transitions with reduced-motion support. Use CSS, no motion dependency.

## Files

- `web/src/api/{types,client}.ts`: stable public types and the single credentialed fetch path with CSRF, abort signals, and `ApiProblem` mapping.
- `web/src/auth/session.tsx`: session bootstrap, visible boot/authentication failures, sign-in, invite redemption, and sign-out.
- `web/src/realtime/socket.ts`: in-memory socket-only sequence cursor, strict event validation/deduplication, stable-open bounded exponential reconnect, and retrying authoritative resync callback.
- `web/src/features/chat/{use-workspace,invalidation,load-pages}.ts`: generation-scoped orchestration, coalesced membership/list invalidation, and bounded authoritative pagination reloads.
- `web/src/features/conversations/collection.ts`: conversation/member append, deduplication, cursor retention, and explicit 500-item caps.
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
- Fetch effects own and abort `AbortController`s on selection/session/socket cleanup. Collections are capped at 500 conversations, 500 members, 200 retained timeline messages, and 20 pending sends.
- Every authored TS/TSX/CSS source is below the requested 300-line soft limit; the largest is 170 lines. `git diff --check` is clean.
- The frontend guidance shaped a cardless working surface: graphite navigation, warm paper timeline, a restrained vermilion accent, rigorous three-pane hierarchy, and CSS-only message/status/drawer transitions with reduced-motion support.

## Concerns

- `npm ci` emits engine warnings because this machine runs Node 23.11 while current Vitest/jsdom dependencies declare supported even-numbered Node releases; install, tests, and build still pass. CI should use a declared supported Node release.
- Conversation/member navigation exposes accessible older-page controls, retains server cursors, deduplicates appended pages, and stops at explicit 500-item caps; message history exposes earlier-page loading until the 200-message client cap. There is no unbounded client accumulation.
- DMs have no server-provided display name in the current contract, so navigation labels them by real conversation ID; the selected right pane shows the real member usernames.
- No threads, reactions, search, media, projects, work items, service worker, offline cache, UI/state/motion framework, or unsupported dependency was added.

## Fix Round 1 evidence

### Direction preserved before fixes

- **Visual thesis:** preserve the calm, text-first workshop: warm paper conversation canvas, graphite navigation, rigorous spacing, and one restrained vermilion thread accent.
- **Content plan:** retain authentication/invite entry, conversation navigation, message timeline/composer, member context, and mobile drawers; add pagination only where the real API exposes it.
- **Interaction thesis:** retain restrained message entrance and fast reduced-motion-aware drawers while making reconnect/resync and keyboard focus transitions operationally exact.

### RED

- `npm --prefix web test -- --run` failed before Round 1 implementation with 9 failing files, 13 failing tests, and one unhandled resync rejection (exit 1).
- Failures directly demonstrated the review findings: missing entity/socket cursor separation, invalid socket payload acceptance, successful reopen after failed resync, upgrade-loop backoff reset, stale selection merges, uncoalesced invalidations, discarded pagination cursors, and drawers without inert/focus/Escape semantics.

### GREEN and verification

- `npm --prefix web test -- --run`: 11 files, 35 tests passed. Round 1 regressions cover HTTP entity merge without cursor advancement; delayed socket seq 8 after an HTTP seq 9 result; invalid payload cursor rejection; failed-then-successful resync; stable-open reconnect timers; selection switches during initial history, send, edit, delete, message pagination, and member pagination; 10,000-event invalidation coalescing; selected-conversation removal; 500-item pagination caps/deduplication; and mobile inert/focus/Escape/restoration behavior.
- `npm --prefix web run build`: passed. Production output is JS 44.74 kB / 15.10 kB gzip, CSS 12.33 kB / 3.47 kB gzip, and HTML 0.39 kB / 0.26 kB gzip. Initial compressed JS remains far below 300 KiB.
- `make check`: passed after a clean `npm ci`; Vite embedded build, `go test -tags sqlite_fts5 ./...`, all 35 Vitest tests, and tagged Go binary build are green.
- `git diff --check`: passed. Every authored TS/TSX/CSS file remains below 300 lines; the largest is `web/src/features/chat/use-workspace.ts` at 265 lines.

### Round 1 implementation review

- HTTP send/edit/delete results now merge by entity and exact pending idempotency key only. They populate per-entity sequence state but never mutate the WebSocket replay cursor; only a validated delivered socket envelope can advance that cursor.
- Selection changes synchronously abort selection/action controllers, advance a generation, and clear detail, members, timeline, pending work, cursors, loading, and errors. Every selected fetch/mutation completion checks the captured conversation ID, generation, and abort signal before merging.
- A failed authoritative resync remains visibly `Sync error`, preserves the socket cursor, and retries with bounded backoff. Cursor reset/reopen occurs only after the complete list/detail/member/history reload succeeds.
- Replayed `conversation.*` invalidations are reduced to one in-flight refresh plus one queued refresh. The refresh includes list and selected detail/members, and immediately clears selection/actions/composer when membership removal makes the selected conversation disappear.
- Conversation and member pagination retains `next_before_id`, provides accessible load controls, appends/deduplicates, and stops at explicit 500-item client caps. Pagination completions are generation/abort scoped.
- Mobile behavior is driven by real `matchMedia` breakpoints. Closed offscreen panes are inert and `aria-hidden`; open drawers trap focus, close on Escape, restore the opener, and move focus into the main workspace after a navigation selection. CSS transitions retain reduced-motion support.
- Reconnect attempts are reset only by the first valid delivered event or an open connection stable for at least 10 seconds. Short upgrade-close loops retain bounded exponential backoff.

### Graphical verification

- The real embedded binary was opened through the connected Chrome extension at 1440×900 and 390×844. Desktop inspection exercised sign-in, public-channel creation, keyboard message send, the three-pane workspace, real member detail, and server-sanitized Markdown presentation: `<strong>` and the safe link rendered while the submitted `<script>` element did not.
- Narrow inspection exercised the closed main surface plus both navigation/context drawers. Browser state confirmed closed panes had `inert` and `aria-hidden="true"`; focus wrapped first-to-last and last-to-first inside navigation; Escape restored each opener. Screenshots were visually checked for clipping, overlap, composer access, drawer width, and focus/layout errors; none were observed on the clean member surface.
- The graphical run exposed and fixed one real-browser-only defect: the native fetch implementation was invoked with `ApiClient` as its receiver, producing the visible network-error state despite a healthy server. A focused regression first failed with the unexpected receiver, then passed after detaching the fetch call; the embedded client subsequently completed the graphical flow.
- Only one connected Chrome profile was discoverable, so a genuine two-window/two-profile graphical session was unavailable. It is not claimed. The earlier production HTTP + WebSocket smoke remains the concurrency evidence: two independent cookie sessions received the same live event and replayed the next sequence exactly after reconnect.

### Remaining concerns

- `npm ci` still reports engine warnings under local Node 23.11 because current Vitest/jsdom packages declare even-numbered supported Node lines; install, tests, builds, and browser validation pass. CI should use Node 22 or 24+ as declared by those packages.
- No Round 1 work touched the three ledgered minor findings or expanded into threads, reactions, search, media, projects/work items, offline/PWA behavior, or new frameworks/dependencies.
