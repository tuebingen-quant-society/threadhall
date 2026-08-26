package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	store "github.com/tuebingen-quant-society/threadhall/internal/store/sqlite"
)

func bootstrapAgent(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("bootstrap-agent", flag.ContinueOnError)
	statePath := flags.String("state-path", "", "required SQLite state path")
	username := flags.String("username", "", "agent username")
	conversationID := flags.Int64("grant-conversation", 0, "conversation initially granted to the agent")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if strings.TrimSpace(*statePath) == "" || !agenttask.ValidUsername(*username) || *conversationID < 0 {
		return fmt.Errorf("state-path and a valid username are required")
	}
	db, err := store.Open(*statePath, 1)
	if err != nil {
		return fmt.Errorf("open persistence: %w", err)
	}
	defer db.Close()
	writer, err := store.NewWriter(db, 2)
	if err != nil {
		return fmt.Errorf("start persistence writer: %w", err)
	}
	defer writer.Close()
	var adminID int64
	if err := db.QueryRow(`SELECT id FROM users WHERE is_admin = 1 AND principal_kind = 'human' ORDER BY id LIMIT 1`).Scan(&adminID); err != nil {
		return fmt.Errorf("find workspace administrator: %w", err)
	}
	token, tokenHash, err := agenttask.NewWorkerToken(rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	service := store.NewAgentStore(db, writer)
	agent, err := service.Create(context.Background(), agenttask.CreateAgent{
		Username: *username, TokenHash: tokenHash, CreatedBy: adminID, CreatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}
	if *conversationID > 0 {
		if err := service.Grant(context.Background(), agenttask.Grant{
			AgentID: agent.ID, ConversationID: *conversationID, CreatedBy: adminID, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("grant conversation: %w", err)
		}
	}
	if _, err := fmt.Fprintf(stdout, "agent_id=%d\nworker_token=%s\n", agent.ID, token); err != nil {
		return fmt.Errorf("write agent credentials: %w", err)
	}
	return nil
}

func setAgentPolicy(arguments []string) error {
	flags := flag.NewFlagSet("set-agent-policy", flag.ContinueOnError)
	statePath := flags.String("state-path", "", "required SQLite state path")
	conversationID := flags.Int64("conversation-id", 0, "conversation to update")
	policy := flags.String("policy", "", "explicit or human_only")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	selected := agenttask.ConversationPolicy(*policy)
	if strings.TrimSpace(*statePath) == "" || *conversationID <= 0 ||
		(selected != agenttask.PolicyExplicit && selected != agenttask.PolicyHumanOnly) {
		return fmt.Errorf("state-path, positive conversation-id, and a valid policy are required")
	}
	db, err := store.Open(*statePath, 1)
	if err != nil {
		return fmt.Errorf("open persistence: %w", err)
	}
	defer db.Close()
	writer, err := store.NewWriter(db, 2)
	if err != nil {
		return fmt.Errorf("start persistence writer: %w", err)
	}
	defer writer.Close()
	var adminID int64
	if err := db.QueryRow(`SELECT id FROM users WHERE is_admin = 1 AND principal_kind = 'human' ORDER BY id LIMIT 1`).Scan(&adminID); err != nil {
		return fmt.Errorf("find workspace administrator: %w", err)
	}
	if err := store.NewAgentStore(db, writer).SetConversationPolicy(
		context.Background(), adminID, *conversationID, selected,
	); err != nil {
		return fmt.Errorf("set agent policy: %w", err)
	}
	return nil
}
