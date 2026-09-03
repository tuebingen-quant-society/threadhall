# Threadhall Next-Value Sequence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the approved collaboration-first sequence as independently releasable slices, from installability through scoped external-agent context.

**Architecture:** The browser remains an HTTP-plus-realtime client of one Go/SQLite authority. Shared knowledge is stored as first-class conversation records, explicit research composes existing message/thread/agent primitives, and MCP receives a separate read-only credential boundary.

**Tech Stack:** Go 1.26, SQLite/FTS5, Preact, TypeScript, Vite, Vitest, Playwright, official MCP Go SDK.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md)

## Global Constraints

- First finish and commit the already in-flight agent activity and default Deep Research plugin work. Do not reimplement or absorb those dirty-tree changes.
- Start each slice from a clean logical checkpoint and keep its schema, service, HTTP, web, and verification changes in one feature commit unless a listed refactor commit is required.
- Resolve the next migration number from `internal/store/sqlite/migrate.go` immediately before implementation. The approved sequence assumes the in-flight `013_message_activity.sql` lands first, so the planned numbers begin at 014.
- Every read and mutation enforces current conversation membership in the SQLite query. Return the same not-found shape for absent and inaccessible conversation data.
- Every durable mutation uses the bounded writer, an idempotency key where retryable, and a durable conversation event in the same transaction.
- Keep user-authored Markdown and snippets untrusted. Render Markdown server-side; never send stored HTML back into an agent prompt.
- Keep new production files below the 300 LOC soft limit. Split models, repositories, handlers, and UI state rather than extending already-large files.
- Do not add mock production data, offline writes, hidden retry loops, generic schemas, repository execution, or agent authority to mutate canonical briefs/decisions.
- These plans supersede only the unchecked search and PWA tasks in `2026-08-25-threadhall-collaboration.md`; they do not silently adopt that older plan's reactions, media, typing, or presence scope.

## Delivery Map

| Order | Slice | Plan | Exit signal |
|---:|---|---|---|
| 0 | In-flight prerequisite | Existing owner/worktree | Activity UI and Deep Research provisioning are committed and all baseline checks pass |
| 1 | Installable PWA | [`2026-09-03-threadhall-installable-pwa.md`](2026-09-03-threadhall-installable-pwa.md) | Install metadata works; shell may load offline; all authoritative data stays visibly offline |
| 2 | Authorized search | [`2026-09-03-threadhall-authorized-search.md`](2026-09-03-threadhall-authorized-search.md) | A result opens and focuses the exact authorized message/thread |
| 3 | Pins | [`2026-09-03-threadhall-pins.md`](2026-09-03-threadhall-pins.md) | Members can pin/unpin messages and agents receive a bounded pin section |
| 4 | Brief | [`2026-09-03-threadhall-conversation-brief.md`](2026-09-03-threadhall-conversation-brief.md) | One revision-checked Markdown brief is editable and enters agent context first |
| 5 | Decisions | [`2026-09-03-threadhall-decisions.md`](2026-09-03-threadhall-decisions.md) | Members can promote and supersede attributable decisions |
| 6 | Research thread | [`2026-09-03-threadhall-research-thread.md`](2026-09-03-threadhall-research-thread.md) | One action atomically starts Deep Research in a named thread |
| 7 | Read-only MCP | [`2026-09-03-threadhall-readonly-mcp.md`](2026-09-03-threadhall-readonly-mcp.md) | A separately scoped token exposes only bounded read tools |

## Sequence Gates

### Task 1: Establish the baseline checkpoint

- [ ] Confirm `git status --short` contains no unowned overlap in `internal/agenttask`, `internal/agentd`, `internal/httpapi/agent_worker*`, `internal/store/sqlite/migrate.go`, or message activity UI files.
- [ ] Run `git log -1 --oneline` and record the exact clean baseline SHA in the execution notes.
- [ ] Run `go test -tags sqlite_fts5 ./...`, `npm --prefix web test -- --run`, `npm --prefix web run typecheck`, and `npm --prefix web run build`; expect success.
- [ ] Record the baseline commit in the first feature commit message body.

### Task 2: Execute slices 1 through 7 in order

- [ ] Complete each linked plan, including its focused tests and commit, before starting the next plan.
- [ ] At each “Add failing tests” checkbox, run the task's focused command immediately and record the expected assertion/compile failure before writing production code.
- [ ] After pins, briefs, and decisions, manually inspect the combined context pane at widths 1440, 980, 700, and 390 pixels.
- [ ] After the research slice, run one fake-runtime integration test. Run a real Codex/Deep Research smoke only when credentials and network access are explicitly available; label it separately from automated coverage.

### Task 3: Run the integrated release gate

- [ ] Run `go test -race -tags sqlite_fts5 ./...`; expect success.
- [ ] Run `npm --prefix web test -- --run`, `npm --prefix web run typecheck`, and `npm --prefix web run build`; expect success.
- [ ] Run the PWA/search/research Playwright specs listed in the slice plans against the production binary; expect success.
- [ ] Run `git diff --check`; expect no whitespace errors.
- [ ] Scan changed files with `rg -n "TODO|FIXME|TBD|placeholder|mock data"`; explain any intentional pre-existing match and remove new placeholders.
- [ ] Confirm `git status --short` contains only intended changes, then commit the release record as `docs: verify Threadhall next-value sequence`.

## Explicit Stop Point

Repository registration, repository search/grounding, worktrees, approval gates, preview servers, commits, pushes, and pull requests are not implementation tasks in this sequence. Start a new approved design before adding any of them.
