# Threadhall Release Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the complete v0.1 safely operable and honestly documented on a low-cost VPS, then publish reproducible amd64/arm64 releases.

**Architecture:** Harden the single-node system around explicit configuration, least-privilege systemd/Caddy deployment, verified migrations/backups, bounded retention jobs, adversarial tests, and measured resource budgets.

**Tech Stack:** Go, SQLite, systemd, Caddy, shellcheck, Playwright, GitHub Actions, govulncheck, npm audit.

**Spec:** [`../specs/2026-08-25-threadhall-design.md`](../specs/2026-08-25-threadhall-design.md)

## Global Constraints

- Do not claim GDPR compliance or performance without operator/legal context and measured evidence.
- Release migration tooling must create and verify a backup before changing a database.
- Deployment grants write access only to `/var/lib/threadhall`; secrets remain owner-readable.
- Security failures are visible and closed; do not silently relax TLS, cookies, permissions, or origin checks.

---

### Task 1: Add privacy and operator documentation

**Files:** Create `docs/privacy/data-map.md`, `retention.md`, `privacy-notice-template.md`, `provider-inventory-template.md`, `data-request-runbook.md`, `backup-restore-erasure.md`, `docs/operations/security.md`; link from `README.md`.

- [ ] Document every data category, purpose placeholder, location, recipients, deletion behavior, and default retention: replay 30 days, logs 14 days, security/agent audit 90 days, pending uploads 24 hours, deleted media seven days, messages until deletion.
- [ ] State prominently that v0.1 does not claim compliance and does not automate access, erasure, portability, lawful basis, DPA/SCC, DPIA, breach reporting, or legal holds.
- [ ] Include plaintext SQLite/FTS disclosure, provider context boundary, encrypted disk/off-host backup requirements, and backup resurrection caveat.
- [ ] Cross-check docs against migrations/config and run a link checker; commit with `docs: add privacy and operator runbooks`.

### Task 2: Implement retention, structured logging, and security headers

**Files:** Create `internal/retention/service.go`, `service_test.go`, `internal/observability/log.go`, `log_test.go`, `internal/httpapi/security.go`, `security_test.go`; modify app lifecycle/config.

- [ ] Add failing tests for bounded batch pruning, configured cutoffs, audit preservation, cancellation, no persisted IP/body/token/provider output, CSP, HSTS behind TLS, frame denial, `nosniff`, referrer policy, and cache headers.
- [ ] Implement one cancellable retention scheduler using small writer transactions and structured redacted logs.
- [ ] Add rate limits for login, invite redemption, messages, search, uploads, sockets, typing/presence, and worker auth with explicit `429` responses.
- [ ] Run race/fuzz tests and commit with `feat(security): enforce retention and transport policy`.

### Task 3: Add verified migration, backup, restore, and integrity commands

**Files:** Create `internal/admin/backup.go`, `backup_test.go`, `restore.go`, `restore_test.go`; modify CLI; create `docs/operations/backup.md`, `scripts/threadhall-backup`.

- [ ] Add failing real-SQLite/filesystem tests for a consistent database-plus-media backup, owner-only output, checksum manifest, insufficient space, corrupt or missing media rejection, migration pre-backup, rollback/refusal, and `PRAGMA integrity_check`.
- [ ] Implement `threadhall admin backup`, `integrity`, and offline `restore --from`; require explicit paths and refuse overwrite unless the documented confirmation flag is present.
- [ ] Ensure startup migration verifies the backup and only then commits schema advancement.
- [ ] Run a real backup/restore drill preserving messages, media bytes/metadata, events, and agent lifecycle; commit with `feat(admin): add verified backup and restore`.

### Task 4: Add hardened systemd and Caddy deployment

**Files:** Create `deploy/threadhall.service`, `deploy/threadhall.env.example`, `deploy/Caddyfile`, `deploy/install.sh`, `deploy/uninstall.md`, `docs/operations/install.md`, `docs/operations/upgrade.md`; create deployment checks.

- [ ] Test install script syntax/idempotency in a disposable Linux VM and validate Caddy/systemd configs.
- [ ] Use a dedicated user, `UMask=0077`, `NoNewPrivileges`, `ProtectSystem=strict`, private temp, capability denial, and only `/var/lib/threadhall` writable; do not embed secrets in unit arguments.
- [ ] Require HTTPS public URL, secure cookies, bounded upload/body timeouts, WebSocket proxying, and explicit state/config paths.
- [ ] Perform clean install, upgrade with verified backup, restart, and uninstall-preserves-state drills; commit with `chore(deploy): add hardened single-node install`.

### Task 5: Build adversarial, load, and resource verification

**Files:** Create `tools/loadtest/main.go`, scenario/client/metrics files and tests, `docs/verification/security.md`, `performance.md`, `backup-restore.md`; extend Playwright tests and Make targets.

- [ ] Test the load tool's deterministic 500-user seed, 200 sockets, 50 message/s pacing, reconnect/replay, latency histogram, timeout, and failure exit codes.
- [ ] Add adversarial cases for IDOR across conversations/media/tasks, CSRF/origin, Markdown/XSS, path traversal, SQL/FTS input, decompression/frame bombs, slow clients, queue saturation, and restart during writes.
- [ ] Run Go race/fuzz tests, frontend tests, Playwright at desktop/mobile widths, `govulncheck ./...`, `npm audit --omit=dev`, and license review; record commands and versions.
- [ ] On an actual CX23-class amd64 host, measure all approved RSS/latency/replay gates; repeat functional smoke on Linux arm64. Mark unmet gates as failures, not caveats.
- [ ] Commit with `test: add security and resource acceptance gates`.

### Task 6: Complete project governance and reproducible releases

**Files:** Create `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `GOVERNANCE.md`, `.github/ISSUE_TEMPLATE/*`, `.github/dependabot.yml`, `.github/workflows/release.yml`; refine `README.md`, CI, Makefile.

- [ ] Document TQS stewardship, AGPL-3.0 contribution terms without a CLA, supported versions, private vulnerability reporting, architecture boundaries, local development, and exact v0.1 limitations.
- [ ] Protect CI with formatting, unit/integration/race, frontend, build, vulnerability, and generated-artifact checks; keep expensive CX23 measurement a documented release gate.
- [ ] Build CGO-enabled Linux amd64/arm64 archives containing binary, license, systemd/Caddy examples, and checksums; test extraction and `threadhall version` in clean Linux environments.
- [ ] Tag `v0.1.0-rc.1`, publish release notes with measured gates and known limitations, and verify installation from the public artifacts.
- [ ] Commit with `chore(release): prepare Threadhall v0.1` before tagging.
