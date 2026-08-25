# Threadhall Agent Collaboration Design

Date: 2026-08-25

Status: Approved in chat; pending written-spec review

## Identity And Invocation

An administrator creates an agent identity and grants it selected conversations and registered
repositories. Passive messages never invoke an agent. Work starts only from an explicit agent
mention, a direct message to the agent, or an explicit task action.

The requester owns the active task and may send unmentioned follow-ups while it runs. Another
member must mention the agent explicitly. A thread has at most one active task and one resumable
runtime session.

## Worker Trust Boundary

`threadhall-agentd` authenticates its outbound TLS connection with a revocable opaque token.
Threadhall stores only the token hash, identity, and scopes. Provider keys, Git credentials,
working copies, and generated artifacts remain on `agentd`; they never enter the chat database
or browser.

Context begins with the invoking message, its thread, permitted attachments, and explicit
references. Additional context uses a bounded authenticated API that rechecks membership and
records the access. Conversation text, attachments, and repository content are delimited as
untrusted input.

## Repository Isolation And Approvals

Each coding thread receives an isolated worktree in a registered repository. Codex starts with
workspace-write sandboxing and approval-on-request. Local analysis, edits, tests, and commits
inside that worktree are allowed.

Network access, pushes, PR creation, merges, destructive commands, and external messages require
an exact administrator approval. An approval stores the task, action type, expiry, sanitized
summary, and action digest. v0.1 supports approve-once and deny; it never grants permanent or
provider-defined authority.

Provider output cannot forge an approval. `agentd` resumes an operation only after Threadhall
sends an authenticated decision that matches the stored interaction and digest.

## Provider-Neutral Protocol

The agent protocol supports:

```text
start(task, context, attachments)
steer(task, message)
interrupt(task)
resume(session)
answer(interaction, response)
events(task)
```

The Codex app-server adapter is implemented first. Later runtimes translate to the same task,
session, progress, approval, question, artifact, failure, and usage events instead of leaking
provider-specific objects into Threadhall.

## Interactive Question Cards

When Codex emits `item/tool/requestUserInput`, `agentd` normalizes the runtime thread, turn,
item, questions, options, and deadline into a durable `agent_interaction`. The task moves to
`waiting_input`, and Threadhall renders one native accessible card inside the task thread.

A card supports at most three questions with:

- single- or multi-select choices;
- confirmation;
- optional other text; or
- bounded short free text.

Only the task owner or an administrator may answer. Threadhall validates the stored schema and
atomically claims the unanswered interaction. Duplicate or stale submissions return `409`.
The accepted non-secret answer becomes a read-only decision summary visible to the thread and
resumes the exact runtime turn.

The runtime deadline is respected. Without one, the card expires after 24 hours and the task
fails visibly rather than inventing an answer. Agents cannot supply HTML, CSS, scripts, or
arbitrary components; Threadhall owns rendering, keyboard behavior, validation, and escaping.
Secret-input requests are rejected in v0.1. Credentials belong in the administrator-managed
`agentd` secret store, not conversation data.

## Lifecycle And Recovery

Critical task, approval, question, and delivery transitions are durable. High-frequency token
and command progress is bounded, sanitized, and coalesced into one temporary card. Raw reasoning,
secrets, full command output, and provider errors are not posted into chat.

An `agentd` disconnect marks active work `interrupted`. A reconnect may resume the recorded
provider session after capability and task checks, but execution never silently restarts. Stop
targets the exact active runtime turn. Final delivery succeeds before the temporary progress card
is removed.
