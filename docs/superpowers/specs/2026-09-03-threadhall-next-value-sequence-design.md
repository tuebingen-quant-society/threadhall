# Threadhall Next-Value Sequence Design

Date: 2026-09-03

Status: Proposed for implementation planning

## Purpose

Threadhall should become useful as a durable team workspace before it grows
into a full remote coding environment. The next sequence therefore strengthens
the existing conversation model first, adds a small shared-knowledge layer,
and only then exposes that state to external agents and repository runtimes.

The product direction is:

```text
conversation
  -> shared context and decisions
  -> research and approved work
  -> optional isolated implementation
  -> verified result returned to the conversation
```

This borrows the collaborative-artifact ideas demonstrated by Chopin and keeps
AQ-style worktrees and persistent execution as a later, optional backend. It
does not turn Threadhall into an MDX editor or browser terminal.

## Goals

- Make important conversation state easy to preserve, find, and reuse.
- Give humans an explicit, attributable way to record decisions.
- Supply agents with stable context without repeatedly copying old messages.
- Compose research from the existing agent, plugin, child-thread, activity,
  and artifact primitives.
- Establish a narrow handoff surface that any MCP-capable agent can consume.
- Keep every slice independently testable and releasable.

## Non-goals

- Rich-text or MDX authoring, CRDTs, live cursors, or collaborative selection.
- Shared terminals, editors, dev-server previews, or managed virtual machines.
- Linear, GitHub pull-request, or multi-harness execution integration in this
  sequence.
- Direct agent mutation of briefs or decisions without a later approval model.
- Human file uploads before the media storage and retention design is built.
- A generic knowledge graph, arbitrary content types, or user-defined schemas.

## Delivery Sequence

Each stage has its own verification and commit boundary. A later stage may use
an earlier one, but no stage should be enlarged to absorb its successor.

1. Finish the in-flight agent activity UI and default Deep Research plugin
   provisioning without duplicating the current working-tree changes.
2. Make the responsive web application installable as a conservative PWA.
3. Add authorized full-text message search.
4. Add message and artifact pins with bounded agent-context inclusion.
5. Add one versioned Markdown brief per conversation.
6. Add attributable, supersedable decision records.
7. Add a research-to-child-thread workflow.
8. Expose a read-only, conversation-scoped MCP context service.
9. Design repository registration and read-only grounding separately.
10. Design worktree execution, approvals, persistence, and PR delivery
    separately after the collaboration path is exercised in real use.

## Stage 1: Existing In-Flight Work

The current working tree already contains changes for structured agent
activity and default Deep Research provisioning. They are a prerequisite, not
part of this feature design. Their owner should finish, verify, and commit them
as an isolated logical change before this sequence modifies overlapping agent
delivery, message, migration, or timeline files.

## Stage 2: Installable PWA

Threadhall will add a web manifest, icons, and a small service worker that
caches only versioned application-shell assets. API, WebSocket, authentication,
and message mutations remain network-authoritative. There is no offline send
queue and no fabricated offline data.

The existing connection state remains the source of truth. Installation and
updates must not hide reconnect, resync, or authentication failures. Desktop
and compact-layout browser tests cover installation metadata, navigation,
update behavior, and explicit offline state.

## Stage 3: Authorized Message Search

SQLite FTS5 indexes current, non-deleted message text. Index changes occur in
the same write transaction as message creation, edits, and deletion. Queries
join through conversation membership so a valid user cannot discover private
channels, direct messages, snippets, authors, or counts they cannot read.

Search uses a bounded query, result count, snippet size, and keyset cursor. The
first UI is one workspace search surface that opens the exact conversation or
child thread and focuses the matching message. Briefs and decisions are added
to search only after their own stages exist; they are not represented as fake
messages.

## Stage 4: Pins

A pin is a reversible conversation-scoped reference to an existing message.
Generated artifacts remain attached to their source message, so pinning that
message also preserves the artifact entry point.

The server stores the conversation, message, actor, and timestamp. It verifies
that the message belongs to the same readable conversation and emits durable
pin-created and pin-removed events. Editing a pinned message updates what the
pin displays because the message identity does not change. Deleting a message
removes its pin in the same transaction so deleted content is not retained
through a secondary surface.

All conversation members may pin and unpin in v1. The context panel shows a
bounded newest-first list with links to the original messages. Agent context
includes a bounded projection of current pins before recent chat, with clear
untrusted-content delimiters and without widening conversation access.

## Stage 5: Conversation Brief

Every channel, private room, direct message, and child thread may have one
optional Markdown brief. It is a separate domain record, not a specially
privileged message. The record contains the raw and sanitized rendered body,
monotonic revision, last editor, and update time.

Any current conversation member may create or edit the brief. Updates require
the expected revision and an idempotency key. A stale edit returns a conflict
with the current revision; the server never silently chooses one writer.

The brief uses the existing Markdown policy and a hard body limit. The UI opens
it in the existing secondary workspace pane with read and edit modes. Agent
context places the current brief before pins and recent chat. Agents may
propose replacement text in an ordinary reply or artifact, but v1 does not let
provider output mutate the brief directly.

## Stage 6: Decisions

A decision is an attributable conversation record with its own stable ID,
statement, actor, timestamp, status, and optional source message. Decisions are
append-oriented: an active decision may be superseded by a new decision, but
its original statement and attribution are not rewritten or deleted through
normal UI flows.

Members may promote an existing message or linked question answer into a
decision. The server copies the bounded statement into the decision record and
retains the source identity; later message edits cannot rewrite what was
decided. The context pane separates active and superseded decisions and links
back to available source messages.

Agent context includes active decisions after the brief and before pins.
Superseded decisions remain visible to humans but do not enter normal agent
context. Automatic promotion of every question answer is deferred; the user
must explicitly choose to record it as a decision.

## Stage 7: Research To Child Thread

The research workflow composes existing primitives instead of creating a new
document engine:

1. A member starts research from a conversation with an exact bounded brief.
2. Threadhall creates a named child thread and an invoking message containing
   the canonical Deep Research plugin reference.
3. The normal agent task publishes structured activity in that child thread.
4. Successful output and generated artifacts remain in the child thread.
5. Humans may pin findings or promote conclusions into parent decisions.

Failure remains visible in the child thread. A failed request does not publish
fake results or silently retry. Parent membership and explicit agent grants
continue to control access; creating a child does not widen private access.

## Stage 8: Read-Only MCP Context Service

Threadhall exposes a bearer-authenticated Streamable HTTP MCP endpoint with a
new read-only credential class. Worker tokens and browser cookies are not
reused. Each credential has explicit conversation grants and a stored token
hash; raw tokens are shown only once.

The first tool set is deliberately small:

- list granted conversations;
- read one conversation brief;
- list pins;
- list active or superseded decisions;
- read one named thread with bounded history; and
- search authorized messages.

Tool discovery reflects the presented credential's scope. Every response has
item and byte limits, never returns provider credentials or filesystem paths,
and applies the same membership boundary as the browser APIs. Mutating tools,
agent invocation, repository access, and implementation reporting are separate
future designs.

## Shared Context Contract

When an explicitly invoked Threadhall agent receives context, the order is:

1. conversation identity and access scope;
2. current brief;
3. active decisions;
4. current pins;
5. invoking message and bounded recent thread or conversation history; and
6. explicit artifacts or references selected for the task.

Every section has independent item and byte limits. Lower-priority history is
trimmed before brief or decision content, but no individual value exceeds its
own hard maximum. User-authored material is labeled as untrusted and cannot
change system instructions or authorization.

## Authorization And Failure Behavior

- Reads and writes recheck current conversation membership server-side.
- Durable mutations use idempotency keys and emit events transactionally.
- Realtime events are filtered through existing conversation authorization.
- Stale brief writes return a visible conflict rather than overwriting.
- Cross-conversation pin sources and decision sources are rejected.
- Deleted messages cannot remain readable through pins or source previews.
- MCP authentication failures reveal no conversation or credential metadata.
- Plugin or research failure produces a visible bounded error and no invented
  child result.

## Verification Strategy

Each stage begins with focused service, SQLite, HTTP, and Preact tests. Every
authorization feature includes an adversarial user who can authenticate but
cannot access the target conversation. Realtime tests cover durable event
replay and duplicate suppression for the new mutations.

The complete sequence retains the existing Go race-tagged suite, frontend
component suite, typecheck, production build, and `git diff --check`. PWA and
search add compact-layout browser acceptance. Research adds a fake-runtime
integration test before an explicitly labeled live Codex and Deep Research
smoke. The MCP stage includes protocol, scope, token-redaction, and bounded
response tests before any live client check.

## Success Criteria

A team can install Threadhall, find prior discussion, pin enduring context,
maintain a concise shared brief, record who made important decisions, ask for
deep research in a child thread, and expose that bounded state to an external
agent without granting it a chat login or repository access. None of these
flows requires a shared terminal, a rich-document runtime, or direct agent
authority to mutate canonical team state.
