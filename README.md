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
./bin/threadhall
```

The service listens on `:8080`; `GET /healthz` returns `ok`. Print its version
with `./bin/threadhall version`. Run the test suite with `make test`.

Threadhall is stewarded by the Tübingen Quant Society and licensed under
[AGPL-3.0](LICENSE).
