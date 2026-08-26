package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func TestAgentStoreClaimsOnlyGrantedBoundedConversationContext(t *testing.T) {
	t.Parallel()
	store, messages, db := newTestAgentStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "admin")
	if _, err := db.Exec(`INSERT INTO conversations(id, kind, name, created_by, idempotency_key, created_at)
		VALUES (2, 'private', 'secret', 1, 'secret', ?);
		INSERT INTO conversation_members(conversation_id, user_id, joined_at) VALUES (2, 1, ?)`, now.Unix(), now.Unix()); err != nil {
		t.Fatalf("seed secret conversation: %v", err)
	}

	token := sha256.Sum256([]byte("codex worker token"))
	agent, err := store.Create(context.Background(), agenttask.CreateAgent{
		Username: "codex", TokenHash: token, CreatedBy: 1, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Grant(context.Background(), agenttask.Grant{
		AgentID: agent.ID, ConversationID: 1, CreatedBy: 1, CreatedAt: now,
	}); err != nil {
		t.Fatalf("Grant: %v", err)
	}

	for _, item := range []struct {
		conversation int64
		body         string
		key          string
	}{
		{1, "Project answer is **42**.", "context"},
		{2, "OUT_OF_SCOPE_SECRET_91D2", "secret"},
		{1, "@codex summarize the answer", "invoke"},
	} {
		mentions := agenttask.MentionedAgents(item.body)
		if _, err := messages.Send(context.Background(), message.SendRecord{
			ConversationID: item.conversation, AuthorID: 1, Body: item.body,
			RenderedBody: "<p>" + item.body + "</p>", IdempotencyKey: item.key,
			Mentions: mentions, CreatedAt: now,
		}); err != nil {
			t.Fatalf("send %s: %v", item.key, err)
		}
	}

	work, found, err := store.Claim(context.Background(), token, now.Add(time.Second))
	if err != nil || !found {
		t.Fatalf("Claim = (%#v, %v, %v)", work, found, err)
	}
	if work.Task.AgentID != agent.ID || work.Task.ConversationID != 1 || len(work.Context) != 2 {
		t.Fatalf("claimed work = %#v", work)
	}
	for _, contextMessage := range work.Context {
		if contextMessage.Body == "OUT_OF_SCOPE_SECRET_91D2" {
			t.Fatal("claim leaked an ungranted conversation message")
		}
	}
	if work.Prompt == "" {
		t.Fatal("claim prompt is empty")
	}
}

func TestAgentStoreHumanOnlyPolicyDeniesQueuedAndFutureWork(t *testing.T) {
	t.Parallel()
	store, messages, db := newTestAgentStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "admin")
	token := sha256.Sum256([]byte("codex worker token"))
	agent, err := store.Create(context.Background(), agenttask.CreateAgent{Username: "codex", TokenHash: token, CreatedBy: 1, CreatedAt: now})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Grant(context.Background(), agenttask.Grant{AgentID: agent.ID, ConversationID: 1, CreatedBy: 1, CreatedAt: now}); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if _, err := messages.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "@codex do not see this", RenderedBody: "<p>invoke</p>",
		Mentions: []string{"codex"}, IdempotencyKey: "invoke", CreatedAt: now,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := store.SetConversationPolicy(context.Background(), 1, 1, agenttask.PolicyHumanOnly); err != nil {
		t.Fatalf("SetConversationPolicy: %v", err)
	}
	if _, found, err := store.Claim(context.Background(), token, now.Add(time.Second)); err != nil || found {
		t.Fatalf("human-only Claim found/error = %v/%v, want false/nil", found, err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM agent_tasks LIMIT 1`).Scan(&state); err != nil || state != "denied" {
		t.Fatalf("queued task state = %q, %v; want denied", state, err)
	}
	if _, err := messages.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "@codex still denied", RenderedBody: "<p>invoke</p>",
		Mentions: []string{"codex"}, IdempotencyKey: "invoke-two", CreatedAt: now,
	}); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM agent_tasks`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("task count = %d, %v; want 1", count, err)
	}
}

func TestAgentStoreCompletesThroughOneAgentAuthoredMarkdownMessage(t *testing.T) {
	t.Parallel()
	store, messages, db := newTestAgentStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "admin")
	token := sha256.Sum256([]byte("codex worker token"))
	agent, err := store.Create(context.Background(), agenttask.CreateAgent{Username: "codex", TokenHash: token, CreatedBy: 1, CreatedAt: now})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Grant(context.Background(), agenttask.Grant{AgentID: agent.ID, ConversationID: 1, CreatedBy: 1, CreatedAt: now}); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	if _, err := messages.Send(context.Background(), message.SendRecord{
		ConversationID: 1, AuthorID: 1, Body: "@codex answer", RenderedBody: "<p>invoke</p>",
		Mentions: []string{"codex"}, IdempotencyKey: "invoke", CreatedAt: now,
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	work, found, err := store.Claim(context.Background(), token, now.Add(time.Second))
	if err != nil || !found {
		t.Fatalf("Claim = (%#v, %v, %v)", work, found, err)
	}
	if err := store.Progress(context.Background(), token, work.Task.ID, "Reading the bounded thread…", now.Add(2*time.Second)); err != nil {
		t.Fatalf("Progress: %v", err)
	}
	completion := agenttask.Completion{TaskID: work.Task.ID, AgentID: agent.ID,
		Output: "## Result\n\n```go\nfmt.Println(42)\n```", RuntimeThreadID: "019d-runtime", CompletedAt: now.Add(3 * time.Second),
		Apps:      []agenttask.InlineApp{{Server: "forms", Tool: "ask", ResourceURI: "ui://forms/ask", HTML: "<form></form>", Arguments: []byte(`{"question":"Choose"}`), Result: []byte(`{"content":[]}`)}},
		Questions: []agenttask.Question{{ID: "scope", Header: "Scope", Question: "Which scope?", Options: []agenttask.QuestionOption{{Label: "Channel", Description: "Current channel"}}}},
	}
	if err := store.Complete(context.Background(), token, completion); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var state, runtimeID, body, rendered string
	var authorID int64
	if err := db.QueryRow(`SELECT t.state, t.runtime_thread_id, m.author_id, m.body, m.rendered_body
		FROM agent_tasks t JOIN messages m ON m.id = t.output_message_id WHERE t.id = ?`, work.Task.ID).
		Scan(&state, &runtimeID, &authorID, &body, &rendered); err != nil {
		t.Fatalf("read completion: %v", err)
	}
	if state != "completed" || runtimeID != "019d-runtime" || authorID != agent.ID || body != completion.Output {
		t.Fatalf("stored completion = %q/%q/%d/%q", state, runtimeID, authorID, body)
	}
	if rendered != "<h2>Result</h2>\n<pre><code>fmt.Println(42)\n</code></pre>\n" {
		t.Fatalf("rendered completion = %q", rendered)
	}
	history, err := messages.History(context.Background(), message.History{ConversationID: 1, UserID: 1, Limit: 50})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(history.Messages[0].InlineApps) != 1 || history.Messages[0].InlineApps[0].ResourceURI != "ui://forms/ask" {
		t.Fatalf("stored inline apps = %#v", history.Messages[0].InlineApps)
	}
	if len(history.Messages[0].Questions) != 1 || history.Messages[0].Questions[0].ID != "scope" {
		t.Fatalf("stored questions = %#v", history.Messages[0].Questions)
	}
	var agentMessages int
	if err := db.QueryRow(`SELECT count(*) FROM messages WHERE author_id = ?`, agent.ID).Scan(&agentMessages); err != nil || agentMessages != 1 {
		t.Fatalf("agent message count = %d, %v; want 1", agentMessages, err)
	}
}

func TestAgentStoreReplacesAndScopesWorkerCapabilities(t *testing.T) {
	t.Parallel()
	store, _, db := newTestAgentStore(t)
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	seedMessageFixtures(t, store.writer, now, []int64{1}, "admin", "outsider")
	token := sha256.Sum256([]byte("codex worker token"))
	agent, err := store.Create(context.Background(), agenttask.CreateAgent{Username: "codex", TokenHash: token, CreatedBy: 1, CreatedAt: now})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Grant(context.Background(), agenttask.Grant{AgentID: agent.ID, ConversationID: 1, CreatedBy: 1, CreatedAt: now}); err != nil {
		t.Fatalf("Grant: %v", err)
	}
	first := []agenttask.Capability{
		{Kind: "plugin", ID: "google-drive@openai-curated-remote", Name: "Google Drive", Description: "Drive files"},
		{Kind: "skill", ID: "better-layout", Name: "Better Layout", Description: "Layout"},
	}
	if err := store.ReplaceCapabilities(context.Background(), token, first, now); err != nil {
		t.Fatalf("ReplaceCapabilities: %v", err)
	}
	if err := store.ReplaceCapabilities(context.Background(), token, first[:1], now.Add(time.Minute)); err != nil {
		t.Fatalf("second ReplaceCapabilities: %v", err)
	}

	visible, err := store.ConversationCapabilities(context.Background(), 1, 1)
	if err != nil || len(visible) != 1 || visible[0].ID != first[0].ID {
		t.Fatalf("visible capabilities = (%#v, %v)", visible, err)
	}
	if _, err := store.ConversationCapabilities(context.Background(), 2, 1); !errors.Is(err, agenttask.ErrNotFound) {
		t.Fatalf("outsider capabilities error = %v, want not found", err)
	}
	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM agent_capabilities WHERE agent_id = ?`, agent.ID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("capability rows = %d, %v; want one replacement row", rows, err)
	}
}

func newTestAgentStore(t *testing.T) (*AgentStore, *MessageStore, *sql.DB) {
	t.Helper()
	db := openTestDB(t)
	writer, err := NewWriter(db, 8)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close() })
	return NewAgentStore(db, writer), NewMessageStore(db, writer), db
}
