# Threadhall Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a public, self-hosted Slack-like team chat for humans and agents that meets the approved low-resource, security, and operability gates.

**Architecture:** A required Go modular monolith owns HTTP, WebSockets, SQLite/FTS5, local media, and an embedded Preact PWA. An optional outbound-connected Go `threadhall-agentd` owns model and Git credentials and adapts Codex app-server into a provider-neutral protocol.

**Tech Stack:** Go, `net/http`, C SQLite/WAL/FTS5, Preact, TypeScript, Vite, Vitest, Playwright, Caddy, systemd, GitHub Actions.

**Spec:** [`../specs/2026-08-25-threadhall-design.md`](../specs/2026-08-25-threadhall-design.md) and [`../specs/2026-08-25-threadhall-agent-design.md`](../specs/2026-08-25-threadhall-agent-design.md)

## Global Constraints

- Keep production required services to `threadhall` and Caddy; `threadhall-agentd` is optional.
- Keep source files under 300 lines where practical; split by domain and transport.
- No ORM, broker, Redis, PostgreSQL, required object storage, or production Node runtime.
- Bound every queue, payload, upload, query, replay window, WebSocket frame, and goroutine fan-out.
- Treat messages, uploads, provider output, and repository content as untrusted.
- Add no fallback, mock production data, speculative provider, or compatibility surface.
- Every mutation is idempotent and commits its durable event in the same SQLite transaction.
- Every authorization test must attempt cross-conversation or cross-task access, not only happy paths.
- Complete a phase's verification and commit checkpoints before starting its successor.

---

## Repository Map

```text
cmd/threadhall/                 required server and admin CLI
cmd/threadhall-agentd/          optional worker
internal/app/                   composition and shutdown
internal/auth/                  users, invites, sessions, CSRF
internal/conversation/          channels, DMs, membership, unread cursors
internal/message/               messages, threads, reactions, search contracts
internal/realtime/              durable events, replay, bounded WebSocket hub
internal/media/                 streaming upload and content-addressed storage
internal/agentprotocol/         versioned server-worker wire contract
internal/agenttask/             grants, tasks, interactions, approvals
internal/store/sqlite/          database, writer, migrations, domain stores
internal/httpapi/               HTTP routing, DTOs, middleware, problem responses
internal/webassets/             embedded production PWA
web/src/                        Preact application and feature modules
tools/loadtest/                 tracked Go load/replay generator
deploy/                         systemd, Caddy, installation examples
docs/privacy/                   operator-facing privacy and data documentation
```

Domain packages define models and narrow store interfaces. They do not import SQLite, HTTP, or WebSocket packages. Transport packages translate DTOs and stable error codes; SQLite packages implement domain stores.

## Execution Order

- [ ] Execute [`2026-08-25-threadhall-core.md`](./2026-08-25-threadhall-core.md): bootstrap, authentication, channels, messages, ordered realtime delivery, and a deployable text-chat UI.
- [ ] Execute [`2026-08-25-threadhall-collaboration.md`](./2026-08-25-threadhall-collaboration.md): threads, reactions, unreads, search, media, reconnect, and responsive PWA behavior.
- [ ] Execute [`2026-08-25-threadhall-agents.md`](./2026-08-25-threadhall-agents.md): agent identities, outbound worker protocol, Codex adapter, worktrees, approvals, and question cards.
- [ ] Execute [`2026-08-25-threadhall-release.md`](./2026-08-25-threadhall-release.md): privacy documentation, hardened deployment, backup/restore, adversarial verification, resource gates, and releases.

## Cross-Phase Acceptance

- [ ] Run `go test -race ./...` and expect all packages to pass.
- [ ] Run `npm --prefix web test -- --run` and expect all component tests to pass.
- [ ] Run `npm --prefix web run build` and verify compressed initial JavaScript is below 300 KiB.
- [ ] Run `npm --prefix web run test:e2e` against a production build and expect desktop/mobile acceptance flows to pass.
- [ ] Run the load tool on a CX23-class host and record: idle server RSS below 128 MiB, server plus Caddy below 384 MiB with 200 sockets, p95 commit-to-fan-out below 100 ms at 50 messages/s, and 10,000-event replay below two seconds.
- [ ] Complete a real backup, restore, `PRAGMA integrity_check`, and post-restore acceptance flow.
- [ ] Build and smoke-test Linux amd64 and arm64 release archives and verify their checksums.

## Commit Boundary

- [ ] After all phase plans pass, update the acceptance records and commit with `docs: record Threadhall v0.1 acceptance`.
