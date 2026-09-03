# Threadhall Installable PWA Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Threadhall installable while preserving the server, WebSocket, and existing connection state as the only authority for conversation data.

**Architecture:** Vite emits a manifest, two fixed icons, and a hand-written service worker. The worker precaches only the hashed build shell and uses network-only handling for `/api/`, `/healthz`, WebSockets, and every non-GET request. A small registration controller reports update availability without fabricating offline state.

**Tech Stack:** Preact, TypeScript, Vite, Vitest, Playwright, Go embedded assets.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 2.

## Global Constraints

- No Workbox dependency, offline mutation queue, cached API response, background sync, or pretend-success UI.
- Use maskable 512×512 and ordinary 192×192 PNG icons plus one SVG source; do not generate runtime icons.
- Cache only same-origin GET requests whose paths are in the build-generated shell list. Delete older `threadhall-shell-*` caches during activation.
- Keep the existing `ConnectionState` label authoritative. PWA code may expose update/install state, never overwrite socket state.

---

### Task 1: Define install metadata and build output

**Files:** Create `web/public/manifest.webmanifest`, `web/public/icons/threadhall.svg`, `web/public/icons/threadhall-192.png`, `web/public/icons/threadhall-512.png`; modify `web/index.html`; create `web/src/pwa/manifest.test.ts`.

- [ ] Add a failing manifest test that reads `public/manifest.webmanifest` and asserts `name`, `short_name`, `start_url: "/"`, `scope: "/"`, `display: "standalone"`, theme/background colors, and both PNG icon sizes including `purpose: "any maskable"` on the 512 icon.
- [ ] Add `link rel="manifest"`, theme color, description, and Apple touch icon metadata to `web/index.html`.
- [ ] Create the icons from the existing Threadhall visual language; verify `file web/public/icons/*.png` reports real 192×192 and 512×512 PNGs.
- [ ] Run `npm --prefix web test -- --run src/pwa/manifest.test.ts`; expect success.
- [ ] Commit checkpoint `feat(web): add PWA install metadata`.

### Task 2: Generate an exact shell asset list

**Files:** Create `web/scripts/write-service-worker.mjs`, `web/scripts/sw-template.js`, `web/src/pwa/service-worker.test.ts`; modify `web/package.json`.

- [ ] Add failing tests that build a temporary asset list and assert the generated worker includes `/`, `/index.html`, the manifest, icons, and every current hashed `/assets/*` file exactly once.
- [ ] Make `build` run Vite and then `node scripts/write-service-worker.mjs`; the script reads `internal/webassets/dist`, substitutes JSON into `sw-template.js`, and writes `internal/webassets/dist/sw.js`.
- [ ] In the worker, implement install precache, activation cleanup, and cache-first responses only for the exact generated GET allowlist. Return `fetch(request)` for all other requests.
- [ ] Add tests proving `/api/v1/session`, `/api/v1/realtime`, `/healthz`, unknown navigation, and POST/PATCH/DELETE are absent from the cache allowlist.
- [ ] Run `npm --prefix web test -- --run src/pwa/service-worker.test.ts` and `npm --prefix web run build`; expect success and a generated `internal/webassets/dist/sw.js`.
- [ ] Commit checkpoint `feat(web): generate conservative app-shell worker`.

### Task 3: Register updates without obscuring failures

**Files:** Create `web/src/pwa/register.ts`, `web/src/pwa/register.test.ts`; modify `web/src/main.tsx`, `web/src/app.tsx`, `web/src/styles.css`.

- [ ] Add failing controller tests for unsupported browsers, successful registration, waiting-worker detection, `updatefound`, explicit reload after `controllerchange`, and a registration error surfaced as a non-blocking status.
- [ ] Export `registerPWA(onState)` returning cleanup and an `activateUpdate()` action. Register only after the `load` event and send `SKIP_WAITING` only after the user clicks Update.
- [ ] Add one compact update banner. Do not add a second offline indicator; retain the existing header connection badge.
- [ ] Add an accessible dismiss action and ensure reduced-motion styles do not animate the banner.
- [ ] Run `npm --prefix web test -- --run src/pwa/register.test.ts src/chat-workspace.test.tsx`; expect success.
- [ ] Commit checkpoint `feat(web): surface PWA updates`.

### Task 4: Set correct embedded-asset caching and browser acceptance

**Files:** Modify `internal/webassets/assets.go`; create `internal/webassets/assets_test.go`, `web/e2e/pwa.spec.ts`; modify `web/playwright.config.ts` and `web/package.json` only if the current branch has not already introduced Playwright commands.

- [ ] If Playwright is still absent, run `npm --prefix web install --save-dev --save-exact @playwright/test@1.62.1`, add `test:e2e: "playwright test"`, and use only Chromium for this slice.
- [ ] Add failing Go tests: hashed `/assets/*` receives `Cache-Control: public, max-age=31536000, immutable`; `/`, `/index.html`, `/sw.js`, and `/manifest.webmanifest` receive `Cache-Control: no-cache`; unknown SPA routes still serve the shell without caching as immutable.
- [ ] Wrap the embedded file server with explicit path-aware headers and SPA fallback; preserve `/api` precedence in `app.New`.
- [ ] Add Playwright acceptance for manifest discovery, service-worker control after reload, update banner behavior, a 390-pixel installed viewport, and an offline reload that shows cached shell plus the real Offline connection status.
- [ ] Run `go test ./internal/webassets ./internal/app`, `npm --prefix web run test:e2e -- pwa.spec.ts`, and `npm --prefix web run build`; expect success.
- [ ] Run `git diff --check` and commit `feat(web): ship installable Threadhall PWA`.
