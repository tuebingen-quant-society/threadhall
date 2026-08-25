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

Threadhall is stewarded by the Tübingen Quant Society and licensed under
[AGPL-3.0](LICENSE).
