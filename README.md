# Threadhall

Threadhall is a tiny self-hosted team chat for humans and agents. It will
provide channels, direct messages, one-level threads, search, files, reactions,
unread state, and explicit agent collaboration in one resource-efficient Go
service backed by SQLite.

The approved
[architecture](docs/superpowers/specs/2026-08-25-threadhall-design.md),
[agent collaboration design](docs/superpowers/specs/2026-08-25-threadhall-agent-design.md),
and [implementation roadmap](docs/superpowers/plans/2026-08-25-threadhall-implementation.md)
are public so implementation decisions remain reviewable.

## Development

Build a runnable binary with embedded production web assets:

```sh
make build
./bin/threadhall serve \
  -state-path ./threadhall.db \
  -public-url http://127.0.0.1:8080 \
  -writer-queue 128 \
  -read-connections 4
```

The service listens on `:8080`; `GET /healthz` returns `ok` only while SQLite is
available. Production additionally requires `-production -secure-cookies` and
an operator-provided HTTPS public URL. Print the version with
`./bin/threadhall version`. Run the tagged SQLite/FTS5 test suite with
`make test`.

## Local Codex teammate

The first agent slice runs Codex outside the web server. A worker identity is a
distinct non-human principal: it cannot use human login, receives only explicit
conversation grants, and stores only a SHA-256 bearer-token hash. Create the
identity after the target conversation exists:

```sh
./bin/threadhall bootstrap-agent \
  -state-path ./threadhall.db \
  -username codex \
  -grant-conversation 1
```

The command prints the worker token once. Keep it outside the repository and
start the outbound worker with an empty, absolute working directory:

```sh
THREADHALL_WORKER_TOKEN='the-one-time-token' \
./bin/threadhall-agentd \
  -threadhall-url http://127.0.0.1:8080 \
  -codex-cwd /absolute/path/to/empty-directory
```

Loopback HTTP is accepted for development; remote workers require HTTPS.
Explicit `@codex` mentions in the granted conversation or its one-level
threads create tasks. Passive messages do not. To enforce a human-only area:

```sh
./bin/threadhall set-agent-policy \
  -state-path ./threadhall.db \
  -conversation-id 1 \
  -policy human_only
```

This policy denies new invocations and cancels queued tasks before a worker can
claim their context. Native question cards, approvals, and file artifacts are
still roadmap work; unsupported interaction requests fail visibly instead of
leaving a permanent progress message. See the
[real Codex verification record](docs/verification/codex-teammate.md).

Threadhall is stewarded by the Tübingen Quant Society and licensed under
[AGPL-3.0](LICENSE).
