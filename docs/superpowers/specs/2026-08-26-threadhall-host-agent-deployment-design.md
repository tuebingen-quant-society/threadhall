# Threadhall Host Agent Deployment Design

Date: 2026-08-26

Status: Draft for final review

## Purpose

Threadhall should be easy to install on one inexpensive Linux VPS while its
Codex teammate can maintain Threadhall and other registered projects from
inside the product. The default deployment therefore separates packaging from
authority:

- the Threadhall application runs through Docker Compose for a predictable
  installation;
- `threadhall-agentd` and Codex run directly on the host as root so explicitly
  requested tasks can inspect and control the dedicated VPS;
- the agent uses the existing bounded Threadhall worker protocol rather than
  sharing the chat database or browser session.

This is an intentionally trusted-machine design. The VPS must be dedicated to
Threadhall because a compromised model, plugin, credential, or administrator
account can control the whole host.

## Goals

- Install a usable HTTPS Threadhall deployment with one command after cloning.
- Keep the always-on chat application small enough for a low-cost VPS.
- Let the host agent edit, test, commit, push, open pull requests, rebuild the
  application, and restart services when the user's request calls for it.
- Let a request choose between applying a change locally and opening a pull
  request; do not impose one workflow on every task.
- Use the current Codex plugin directory, repo marketplaces, Agent Skills, MCP
  servers, and inline MCP App UI instead of creating a Threadhall-only format.
- Seed useful Tuebingen Quant Society workflows without pre-authorizing broad
  third-party access.

## Non-goals

- Kubernetes, clustering, or a multi-host control plane.
- A general-purpose remote root shell that bypasses the agent task model.
- Running the default agent inside the Threadhall container.
- Protecting unrelated workloads from a root-level Threadhall agent.
- Installing or authenticating every available integration by default.
- Silently deploying a change when the user explicitly requested only a pull
  request, review, or proposal.

## Runtime Topology

```text
Dedicated VPS

Browser
   |
   v
Docker Compose
|- caddy                 HTTPS termination
`- threadhall            Go API, embedded web UI, SQLite, files
       ^
       | localhost worker HTTP with bearer token
       v
Host systemd
`- threadhall-agentd     root-owned worker supervisor
       `- codex app-server
           |- registered repositories and task worktrees
           |- git and optional GitHub credentials
           |- docker compose
           `- systemctl and host diagnostics
```

The Threadhall container publishes its application port only on loopback for
Caddy and the host worker. `threadhall-agentd` publishes no inbound port. It
claims bounded tasks from Threadhall and posts progress, interactions, and
results back through authenticated HTTP.

## Installation And Files

The repository ships `compose.yaml`, production container definitions, a
systemd unit, and an idempotent `install.sh`. The intended operator flow is:

```sh
git clone https://github.com/tuebingen-quant-society/threadhall
cd threadhall
sudo ./install.sh
```

The installer:

1. verifies a supported Linux host and Docker Compose;
2. creates `/opt/threadhall` for the deployment checkout;
3. creates `/var/lib/threadhall` for SQLite, media, backups, and agent state;
4. creates owner-readable configuration under `/etc/threadhall`;
5. builds and starts Caddy and Threadhall;
6. installs Codex and `threadhall-agentd` on the host;
7. bootstraps the agent identity and stores its one-time worker token outside
   the repository;
8. installs and enables `threadhall-agentd.service`;
9. guides Codex and optional GitHub authentication; and
10. verifies `/healthz`, agent connectivity, and plugin catalog discovery.

Re-running the installer reconciles the installation without replacing
conversation state, credentials, or plugin data.

## Project And Self-maintenance Model

An administrator registers repositories with an identifier, canonical path,
remote, and allowed operations. The Threadhall checkout is registered as the
special `threadhall/self` project. A coding task names one registered project;
the worker rejects arbitrary paths supplied through chat.

Each coding task uses a dedicated Git branch and worktree. Codex starts in that
worktree with workspace-write access. Because the worker runs as root, Codex
can perform host operations when the task requires them; this is an explicit
property of this deployment mode rather than a sandbox guarantee.

Requests determine delivery behavior:

- **Update and apply:** edit, test, build, commit, rebuild the affected
  container or binary, restart it, and report the installed commit.
- **Create a pull request:** edit, test, commit, push, create the pull request,
  and report its URL without deploying it.
- **Apply a pull request or commit:** fetch the named revision, verify and test
  it, rebuild, restart, and report the installed revision.
- **Restart Threadhall:** restart the application container and confirm its
  health endpoint returns successfully.
- **Restart the agent:** schedule a short delayed systemd restart, publish the
  final status, and allow systemd to start a fresh worker process.
- **Update the agent:** build and atomically install a replacement binary,
  publish the result, and schedule the systemd restart.

A container rebuild completes before `docker compose up` replaces the running
application. Failed builds or tests leave the current service running. The
worker records enough task state for a restart to expose an interrupted or
completed outcome instead of losing the task silently.

## Plugin And Skill Model

OpenAI documents one public plugin directory shared by ChatGPT and Codex.
Plugins may contain skills, an MCP server, or both, and an MCP server may
return optional UI resources. Codex also supports local and repository
marketplaces through `codex plugin marketplace` and `codex plugin add`.

Threadhall does not mirror or fork those marketplaces. `threadhall-agentd`
asks the Codex App Server for the enabled installed plugin and skill catalog,
then publishes the bounded catalog to conversations granted to that agent.
The composer exposes `@agent`, `/plugin`, and `/skill` completion, while MCP
App resources render through Threadhall's sandboxed inline UI host.

The open [Agent Plugins 1.0 specification](https://agent-plugins.org/specification)
provides a portable `plugin.json`, `skills/`, and `mcp.json` package. ChatGPT
and Codex are listed as compatible clients for Agent Skills plus stdio and
Streamable HTTP MCP servers. The current
[OpenAI plugin packaging documentation](https://developers.openai.com/plugins/build/plugins)
uses `.codex-plugin/plugin.json` plus `.mcp.json` for Codex marketplaces.
TQS-authored packages therefore share one `skills/` tree and include both the
portable manifest and the small Codex compatibility manifests until one layout
is sufficient across all supported clients.

### Seeded TQS Marketplace

The repository-scoped `threadhall-team` marketplace is installed during host
setup. It contains three small plugins installed by default:

1. **threadhall-maintainer**
   - diagnose the running installation;
   - implement and apply a self-update;
   - restart or update the host worker;
   - back up, verify, and restore Threadhall state.
2. **tqs-engineering**
   - turn an issue or conversation into a tested change;
   - review a diff and run focused regression checks;
   - prepare a release or pull request.
3. **tqs-operations**
   - produce a project status update;
   - turn a meeting into decisions and actions;
   - prepare an event plan;
   - prepare a sourced research brief.

These packages contain instructions, scripts, and templates but no secrets.
They reuse tools already available to Codex and add an MCP server only when a
controlled action or inline UI genuinely requires one.

The OpenAI public directory and additional Git marketplaces remain available
to administrators. GitHub, Google Workspace, Notion, Slack, Sentry, finance,
and research integrations are optional installations because each introduces
credentials, permissions, dependencies, or external data. The installer never
bakes those credentials into an image or repository.

## Resource Policy

The default worker runs at most one Codex task at a time. Task output, command
logs, plugin payloads, and worktrees remain bounded. Finished worktrees and old
images are cleaned on a documented schedule. The release must publish measured
idle and active memory, CPU, disk, and build-time results before claiming a
specific VPS size. A remote host agent remains possible without changing the
Threadhall application when local builds exceed the machine's resources.

## Failure Behavior

- When Threadhall is unavailable, the worker reconnects without inventing task
  completion.
- When the worker is unavailable, messages remain usable and agent tasks remain
  queued or visibly interrupted.
- A failed build or health check leaves the previous application running.
- A failed agent restart is visible through systemd status and Threadhall's
  agent presence state.
- Git push or pull-request failures do not convert into a local deployment.
- Plugin discovery failures are reported per plugin and do not hide healthy
  capabilities.

## Verification

Automated and live checks cover:

- a fresh supported Linux VM installation and a second idempotent run;
- persistent state across container rebuilds and host reboots;
- an `@codex` self-change that edits, tests, rebuilds, restarts, and reports the
  installed commit;
- a pull-request-only task that never changes the running installation;
- Threadhall and worker restart commands, including interrupted task recovery;
- a failed candidate build that leaves the old service healthy;
- TQS marketplace installation and catalog visibility;
- portable and Codex manifest validation for every seeded plugin;
- a seeded skill invocation and an MCP App inline UI invocation; and
- resource measurements with one active Codex task.

## Acceptance Story

An administrator provisions a small dedicated VPS, clones Threadhall, and runs
one installer command. Threadhall opens over HTTPS and its host Codex teammate
appears online with the seeded TQS skills. During normal use, a member notices
workflow friction and asks the teammate to improve Threadhall. The teammate
works in the registered repository, shows progress, tests the change, and
either opens a GitHub pull request or rebuilds and restarts the installation as
requested. Conversation data survives, the teammate reconnects after its own
restart, and the result links to the exact commit or pull request.
