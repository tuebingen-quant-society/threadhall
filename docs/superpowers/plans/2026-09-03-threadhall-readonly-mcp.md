# Threadhall Read-Only MCP Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let external MCP-capable agents read explicitly granted Threadhall context without receiving a browser session, worker token, write tool, repository path, or provider credential.

**Architecture:** A new credential domain hashes independent bearer tokens and grants them to conversations. An authenticated stateless Streamable HTTP MCP handler creates a request-scoped server backed by bounded read queries. It exposes six fixed tools whose data is filtered by the presented credential.

**Tech Stack:** Go 1.26, SQLite, `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0, Streamable HTTP, existing search/knowledge/message stores.

**Spec:** [`../specs/2026-09-03-threadhall-next-value-sequence-design.md`](../specs/2026-09-03-threadhall-next-value-sequence-design.md), Stage 8. The pinned SDK release supports stateless Streamable HTTP and protocol `2026-07-28`; see the [official release](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0) and [transport documentation](https://github.com/modelcontextprotocol/go-sdk/blob/v1.7.0/docs/protocol.md).

## Global Constraints

- Context tokens are a new principal class. Never store raw tokens, reuse browser cookies/CSRF, accept agent-worker tokens, or create hidden human users.
- Use stateless Streamable HTTP with JSON responses. Bearer authentication runs before MCP request parsing and returns the same 401 for absent, malformed, unknown, or revoked tokens.
- Request body limit is 1 MiB. Each tool returns at most 100 items and 256 KiB encoded JSON; tool-specific limits below are smaller.
- No mutation, invocation, repository, filesystem, provider-secret, raw token, HTML, or admin tool is registered.

---

### Task 1: Add separately scoped context credentials

**Files:** Create `internal/contextauth/model.go`, `internal/contextauth/store.go`, `internal/contextauth/service.go`, `internal/contextauth/service_test.go`.

- [ ] Add failing tests for label validation (1–80 bytes), empty/duplicate grants, nonpositive IDs, random-token generation failure, hashing, one-time raw token return, revocation, and clock propagation.
- [ ] Define `Credential { ID int64; Label string; CreatedAt time.Time; RevokedAt *time.Time }`, `Created { Credential; Token string }`, `Create { AdminID int64; Label string; ConversationIDs []int64 }`, and `Principal { CredentialID int64; ConversationIDs []int64 }`.
- [ ] Generate 32 random bytes and encode base64url without padding. Pass only SHA-256 to the repository and zero/release raw byte buffers after forming the one response.
- [ ] Define repository create/authenticate/revoke methods. Authentication accepts a candidate hash and returns only active grants.
- [ ] Run `go test ./internal/contextauth`; expect success.
- [ ] Commit checkpoint `feat(mcp): define read-only context credentials`.

### Task 2: Persist credentials and explicit grants

**Files:** Create `internal/store/sqlite/migrations/019_context_credentials.sql`, `internal/store/sqlite/context_credential_store.go`, `internal/store/sqlite/context_credential_store_test.go`; modify `internal/store/sqlite/migrate.go`, `internal/store/sqlite/migrate_test.go`.

- [ ] Add failing tests for admin-only create/revoke, token-hash uniqueness, atomic multi-grant creation, nonexistent conversation rollback, authentication, constant-time candidate comparison, revoked denial, conversation delete cascade, and no raw-token column.
- [ ] Add `context_credentials(id, label, token_hash, created_by, created_at, revoked_at)` and `context_credential_grants(credential_id, conversation_id, created_by, created_at)` with strict constraints and cascade from conversations/credentials.
- [ ] Authenticate by reading active candidate hashes and using `subtle.ConstantTimeCompare`, matching the existing agent-token boundary.
- [ ] Recheck grants on every request rather than caching scope across requests.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/contextauth ./internal/store/sqlite`; expect success.
- [ ] Commit checkpoint `feat(mcp): persist scoped context credentials`.

### Task 3: Add local create/revoke administration

**Files:** Modify `cmd/threadhall/main.go`, `cmd/threadhall/main_test.go`; create `cmd/threadhall/context_token.go`, `cmd/threadhall/context_token_test.go`.

- [ ] Add failing CLI tests for required state path/admin username/label/grants, non-admin/missing admin, nonexistent conversation, duplicate grant normalization, one-line token output, no token in errors, and revoke by credential ID.
- [ ] Add `create-context-token --state-path PATH --admin USER --label LABEL --conversation ID` (repeatable) and `revoke-context-token --state-path PATH --admin USER --credential-id ID`.
- [ ] Print credential ID, label, and raw token exactly once on successful create; make stderr/errors omit the token. Revocation prints only the credential ID.
- [ ] Do not add a browser token-management surface in this slice.
- [ ] Run `go test ./cmd/threadhall`; expect success.
- [ ] Commit checkpoint `feat(cli): manage MCP context tokens`.

### Task 4: Build bounded context read queries

**Files:** Create `internal/contextreader/model.go`, `internal/contextreader/store.go`, `internal/contextreader/service.go`, `internal/contextreader/service_test.go`, `internal/store/sqlite/context_reader.go`, `internal/store/sqlite/context_reader_test.go`.

- [ ] Add failing tests for granted-conversation listing, brief/pin/decision reads, active-only versus include-superseded, exact named-thread resolution, duplicate/missing thread names, delegated authorized search, and every item/byte limit.
- [ ] Define methods `ListConversations`, `ReadBrief`, `ListPins`, `ListDecisions`, `ReadNamedThread`, and `SearchMessages`; every input begins with `CredentialID` and an optional conversation ID must be grant-checked in SQL.
- [ ] List at most 100 granted conversations; briefs at most 32 KiB; pins 50; decisions 100; named-thread history 100 messages/128 KiB; search 25 hits. Return raw Markdown/text plus attribution, never rendered HTML.
- [ ] Resolve named threads by exact case-insensitive title within one granted conversation. Return a conflict for duplicate titles and not-found for missing/inaccessible titles.
- [ ] Delegate search normalization to `search.Service` while using credential-grant joins rather than inventing a human user ID.
- [ ] Add a final encoded-response limiter that replaces oversize success with a bounded tool error.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/contextreader ./internal/store/sqlite`; expect success.
- [ ] Commit checkpoint `feat(mcp): add bounded context reads`.

### Task 5: Expose six stateless MCP tools

**Files:** Modify `go.mod`, `go.sum`, `cmd/threadhall/main.go`; create `internal/mcpcontext/server.go`, `tools.go`, `auth.go`, `server_test.go`, `tools_test.go`.

- [ ] Run `go get github.com/modelcontextprotocol/go-sdk@v1.7.0`; verify only the intended module graph changes.
- [ ] Add failing SDK client and raw HTTP tests for authentication before parsing, initialize/discovery, `tools/list`, all tool schemas, successful calls, invalid arguments, denied conversation, revoked token, 1-MiB request rejection, 256-KiB response guard, and token redaction.
- [ ] Register one `mcp.NewStreamableHTTPHandler` at `/mcp` with `StreamableHTTPOptions{Stateless:true, JSONResponse:true}`. The request factory constructs a server bound to the authenticated `contextauth.Principal`.
- [ ] Register exactly `list_conversations`, `read_brief`, `list_pins`, `list_decisions`, `read_thread`, and `search_messages` via `mcp.AddTool`; use closed typed input structs and reject unknown JSON fields where the SDK permits validation hooks.
- [ ] Return JSON as MCP structured output plus a compact text content block for compatibility. Mark tool failures as errors without exposing SQL, tokens, grants, or existence outside scope.
- [ ] Apply `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, origin validation against configured public origin, and the request body limit outside the SDK handler.
- [ ] Run `go test -race -tags sqlite_fts5 ./internal/mcpcontext ./internal/contextauth ./internal/contextreader ./cmd/threadhall`; expect success.
- [ ] Commit checkpoint `feat(mcp): expose read-only Threadhall context`.

### Task 6: Verify with two credentials and an external client

**Files:** Create `docs/verification/readonly-mcp.md`, `scripts/smoke-mcp-context.sh`.

- [ ] Make the smoke script accept endpoint/token/conversation arguments, send no secrets to stdout, and call discovery plus each read tool. Keep it read-only and fail on any MCP error result.
- [ ] In an automated integration test, create credentials A and B with disjoint grants; prove A cannot infer B's conversation through list, direct read, thread name, search result, count, or error detail.
- [ ] Run the script against a local production binary with a temporary database and record protocol version, SDK version, tool list, bounded result counts, and redacted outcomes in the verification document.
- [ ] Run `go test -race -tags sqlite_fts5 ./...`, `npm --prefix web test -- --run`, `npm --prefix web run typecheck`, `npm --prefix web run build`, and `git diff --check`; expect success.
- [ ] Scan with `rg -n "context token|Authorization|Bearer|token_hash" docs/verification internal/mcpcontext` and confirm no raw token fixture or runtime token appears.
- [ ] Commit `feat(mcp): ship scoped read-only context service`.
