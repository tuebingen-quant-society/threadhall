# Threadhall Architecture Design

Date: 2026-08-25

Status: Approved in chat; pending written-spec review

Steward: Tübingen Quant Society

License: AGPL-3.0

## Product Definition

Threadhall is a tiny self-hosted team chat for humans and agents. It provides the useful
information model of Slack—channels, direct messages, one-level threads, search, files,
reactions, and unread state—without enterprise surface area or a distributed runtime.
Agents join as first-class team members, receive explicit channel and repository grants,
and work through visible, interruptible, approval-gated tasks.

The public repository will be `tuebingen-quant-society/threadhall`. The product is a
standalone open-source project stewarded by TQS rather than a TQS-branded internal tool.

## Goals And Constraints

- Run the complete chat service comfortably on a Hetzner CX23-class instance: two shared
  vCPUs, 4 GiB RAM, and 40 GB disk.
- Prove 500 registered users and 200 simultaneously online users on one node.
- Keep the required deployment to one Threadhall process, one SQLite database, local media,
  and Caddy for TLS.
- Ship a responsive installable web application; require no native client or Node runtime in
  production.
- Keep all queues, payloads, uploads, queries, goroutines, and replay windows bounded.
- Use direct, inspectable implementations. Avoid an ORM, message broker, Redis, PostgreSQL,
  Kubernetes, and mandatory object storage.
- Keep files below 300 lines as a soft boundary signal. Split transport, domain, and storage
  responsibilities instead of accumulating large service files.
- Treat chat messages, attachments, provider output, and repository content as untrusted.

## Explicit Non-Goals For v0.1

- Multiple workspaces, federation, horizontal clustering, or compatibility with Slack APIs
- Voice, video, calls, huddles, screen sharing, or media transcoding
- Native desktop or mobile clients, push notifications, or offline message creation
- End-to-end encryption or field-level message encryption
- Per-message read receipts, custom emoji, stickers, rich unfurls, or history imports
- Workflow builders, a generic bot marketplace, scheduled agents, or synthesized agent memory
- Automated GDPR compliance, legal-basis decisions, DPA/SCC generation, DPIA automation,
  breach reporting, legal holds, or data-subject-request automation

## Considered Architectures

### Selected: Go Modular Monolith With SQLite

One Go process owns HTTP, WebSockets, authorization, chat domains, event fan-out, SQLite,
FTS5, and local media. An optional agent worker is a separate binary from the same repository.
This directly optimizes the constrained deployment while preserving domain seams that can be
replaced later.

### Rejected: Rust Modular Monolith

Rust with Axum, Tokio, and SQLite could provide tighter allocation control, but the workload
is dominated by network and storage waits. It would slow implementation and raise the
contributor barrier without materially improving the target deployment.

### Rejected: Scale-First Services

PostgreSQL, Redis or NATS, object storage, and separate API/realtime workers would ease future
multi-node operation at the cost of idle resources, operations, and failure modes that the
first 500 users do not require.

Forking Mattermost, Matrix, or Zulip was also rejected. Their inherited protocols, database
assumptions, administrative surfaces, and dependency weight conflict with Threadhall's core
experiment.

## Technology Choices

- Go with `net/http` for the server and administrative CLI
- C SQLite through `database/sql`, compiled with WAL and FTS5 support
- Small WebSocket dependency with explicit queue and frame limits
- Preact, TypeScript, and Vite for the responsive PWA
- Safe server-side Markdown parsing and HTML sanitization
- Embedded migrations and embedded production web assets
- Caddy and systemd for the canonical production deployment

No Node process, database server, broker, or agent runtime is required for ordinary chat.

## Process And Module Boundaries

`threadhall` is the required binary. It serves the PWA and API, authenticates humans,
persists chat state, and owns browser WebSockets.

`threadhall-agentd` is optional. It owns model credentials, Codex app-server processes, Git
credentials, registered repositories, isolated worktrees, and generated artifacts. It makes
an authenticated outbound WebSocket connection to Threadhall, so a private worker or Jetson
needs no public callback port.

Initial repository boundaries are:

```text
cmd/threadhall
cmd/threadhall-agentd
internal/auth
internal/conversation
internal/message
internal/media
internal/realtime
internal/agenttask
internal/store/sqlite
internal/agentprotocol
web/
```

Domain packages expose narrow interfaces and do not import HTTP, WebSocket, or SQLite types.
The SQLite and in-process hub implementations satisfy those interfaces without pretending the
system is distributed.

## Conversation Model

A single `conversations` abstraction represents public channels, private channels, and
one-to-one direct messages. Membership controls every history, replay, search, media, and
agent-context operation.

Replies point directly to a root message through `thread_root_id`; nested reply trees are not
supported. Message edits preserve the identifier and set `edited_at`. Deletion leaves a
tombstone so ordering and replies remain stable. User/conversation membership stores one
monotonically increasing read cursor; Threadhall does not write a receipt per message.

Core tables are:

```text
users, sessions
conversations, conversation_members
messages, attachments, reactions
agent_identities, agent_channel_grants
agent_tasks, agent_sessions, agent_task_events, agent_interactions
events
```

Server entities use compact 64-bit integer identifiers. Client-generated idempotency keys
make retried mutations safe.

## SQLite Write Discipline

SQLite runs in WAL mode with `synchronous=FULL`. Reads use a small connection pool. Every
mutation passes through one bounded writer path, which provides deterministic ordering and
natural backpressure. A saturated writer returns a retryable `503`; it does not grow an
unbounded queue or rely on arbitrary busy retries.

Every user-visible mutation writes its domain rows and one compact event in the same
transaction. FTS5 changes happen in that write path. Critical agent lifecycle transitions are
durable; token-level progress is coalesced and ephemeral.

## HTTP And Realtime Protocol

HTTP handles commands, uploads, pagination, history, search, and resynchronization. WebSockets
carry ordered server events, agent activity, typing, and presence. This keeps mutation
idempotency and error responses independent of connection state.

Every durable event receives a global `events.seq` cursor and a small JSON envelope. On
reconnect, the server registers the subscriber, captures the database high-water mark, replays
authorized events through that mark, and then forwards newer hub events while filtering
duplicates. This ordering closes the database-query/subscription race.

Each client socket has strict event-count and byte budgets. Slow clients are disconnected and
resynchronize instead of blocking fan-out. Events older than the configured 30-day default are
pruned. A cursor older than the retained minimum receives `resync_required` and reloads
authoritative state over HTTP. Typing and presence are rate-limited, memory-only TTL signals.

## Media

Uploads stream through a hard size limiter into an owner-only temporary file. Threadhall
sniffs MIME type, computes SHA-256 while streaming, and atomically renames the file into a
content-hash path. SQLite stores metadata, ownership, state, and message references, never the
media bytes.

A pending attachment token is claimed by a message transaction. Unclaimed uploads expire
after 24 hours. Deleted media becomes inaccessible immediately and is removed after a seven-day
grace period. The default workspace media quota is 10 GiB. v0.1 serves accepted originals and
uses native browser image/video support; it never transcodes.

## Agent Collaboration

Agents are explicit, scoped team members rather than passive readers. Agent execution,
credentials, context retrieval, repository isolation, external-action approvals, provider
normalization, and interactive question cards follow the companion
[`2026-08-25-threadhall-agent-design.md`](./2026-08-25-threadhall-agent-design.md).
The Codex adapter ships first behind a provider-neutral protocol.

## Human Authentication And Authorization

The first administrator is created by a one-time CLI bootstrap command. Accounts are
invite-only. Username is required; email, display name, and avatar are optional. Passwords use
Argon2id. Browser sessions use rotated opaque tokens stored hashed server-side and cookies with
`HttpOnly`, `Secure`, and `SameSite`. Mutations use CSRF protection; WebSockets enforce origin
and session checks.

v0.1 roles are `admin` and `member`. Authorization is applied at the query and event-filtering
boundaries, not only in the UI. Approval decisions are accepted only from authenticated UI
events linked to a stored task, exact sanitized action summary, digest, and expiry. v0.1 offers
approve-once and deny, never blanket approval.

Markdown is sanitized, filenames are never interpreted as paths, uploads are content-sniffed,
responses use a strict Content Security Policy, and public errors never include raw SQLite,
filesystem, or provider details.

## Privacy And GDPR-Enabling Posture

Threadhall adopts privacy-friendly technical defaults but does not claim that v0.1 or a
self-hosted installation is GDPR-compliant. The instance operator remains responsible for its
purposes, lawful basis, notices, processor agreements, transfer safeguards, data-subject
requests, and organizational controls.

v0.1 stores no analytics, advertising identifiers, or persisted IP addresses. It requires only
necessary session cookies. Agent invocation identifies the configured runtime/data recipient
before context leaves the instance. Provider access is explicit and audited. TLS is mandatory,
tokens are hashed, and deployment documentation requires encrypted disks and encrypted off-host
backups. SQLite messages and FTS content remain plaintext on the server filesystem; this is
documented without ambiguity.

The repository includes a data map, retention table, privacy-notice template, agent-provider
inventory template, data-request runbook, and restore/erasure caveat. Default retention is 30
days for replay events, 14 days for application logs, 90 days for security/agent audit, 24 hours
for pending uploads, and seven days for deleted media. Messages remain until users or operators
delete them.

Self-service export, automated anonymization, and purge tooling are deferred. Documentation
must state that v0.1 does not automate access, erasure, or portability requests and that
restoring an old backup may resurrect changes or erasures made after that snapshot.

## Failure And Recovery Semantics

Public API errors use stable machine-readable codes. Invalid input is `400`, unauthenticated
access `401`, unauthorized access `403`, conflicts and stale answers `409`, upload limits `413`,
rate limits `429`, and saturated/transient dependencies `503`.

Agent disconnects transition active tasks to visible `interrupted` state. Reconnect may resume
the recorded session, but execution never silently restarts. Progress cards become explicit
error cards when delivery or execution fails. Database startup checks migrations and schema
version; unknown newer schemas are refused. Migrations create a verified backup before moving
forward.

## MVP Vertical Slices

1. Bootstrap, login, invite, channel creation, and realtime text
2. Threads, reactions, unread cursors, reconnect replay, and authorized search
3. Streaming uploads, image/file presentation, cleanup, and quotas
4. Agent identity, outbound `agentd`, Codex task, progress, stop, and recovery
5. Worktrees, approvals, artifacts, and interactive question cards
6. Responsive PWA, mobile hardening, accessibility, privacy docs, and release gates

Every slice must be deployable and testable before the next begins.

## Verification And Resource Gates

- `threadhall` idle RSS below 128 MiB
- Threadhall plus Caddy below 384 MiB with 200 connected WebSockets
- p95 message commit-to-fan-out below 100 ms at 50 messages per second
- 10,000-event replay below two seconds
- Initial compressed JavaScript below 300 KiB
- No unbounded goroutine, queue, query, event payload, or upload behavior
- Linux amd64 and arm64 release artifacts

Gates are measured on an actual CX23-class instance before being claimed.

Verification includes Go unit tests, real-SQLite integration tests, migration tests, the race
detector, protocol contract tests, fuzzing of parsers and paths, frontend component tests,
Playwright desktop/mobile flows, adversarial authorization tests, three-browser reconnect
tests, a tracked Go load generator, dependency/security scans, and a backup/restore/integrity
drill.

## Deployment And Stewardship

The canonical deployment is a release tarball under systemd with Caddy. State lives under
`/var/lib/threadhall`; one TOML file holds non-secret configuration and generated secrets stay
owner-readable. The systemd service uses a dedicated user, strict filesystem access,
`NoNewPrivileges`, and only the state directory writable. Container images may be published for
convenience but do not define the architecture.

The public repository includes AGPL-3.0, README, SECURITY, CONTRIBUTING, a Code of Conduct,
issue templates, architecture and privacy documentation, and concise governance naming TQS as
steward. No CLA is required initially. CI protects `main`; releases produce checksummed amd64
and arm64 artifacts.

## Acceptance Story

On a clean low-cost VPS, an administrator installs Threadhall, creates an invite, and two users
join from separate browsers. They exchange ordered messages, open a thread, react, upload an
image, disconnect, reconnect without gaps, and search only authorized history. One user mentions
a remotely connected Codex agent. The agent works in an isolated repository worktree, displays
bounded progress, asks a structured question in a native card, resumes from the authenticated
answer, requests approval for an external mutation, and returns its final message and artifact.
Backup, restore, integrity, three-browser, privacy-boundary, and 500/200 load checks pass within
the documented resource budgets.
