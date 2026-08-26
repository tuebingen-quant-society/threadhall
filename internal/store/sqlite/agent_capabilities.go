package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

const maxAgentCapabilities = 512

func validCapability(item agenttask.Capability) bool {
	return (item.Kind == "plugin" || item.Kind == "skill") &&
		len(item.ID) > 0 && len(item.ID) <= 256 && strings.TrimSpace(item.ID) == item.ID && utf8.ValidString(item.ID) &&
		len(item.Name) > 0 && len(item.Name) <= 128 && strings.TrimSpace(item.Name) == item.Name && utf8.ValidString(item.Name) &&
		len(item.Description) <= 1024 && utf8.ValidString(item.Description)
}

func (s *AgentStore) ReplaceCapabilities(ctx context.Context, tokenHash [32]byte, items []agenttask.Capability, now time.Time) error {
	if len(items) > maxAgentCapabilities {
		return agenttask.ErrInvalidInput
	}
	for _, item := range items {
		if !validCapability(item) {
			return agenttask.ErrInvalidInput
		}
	}
	err := s.writer.Do(ctx, func(tx *sql.Tx) error {
		agentID, err := authenticatedAgent(ctx, tx, tokenHash)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM agent_capabilities WHERE agent_id = ?`, agentID); err != nil {
			return err
		}
		for _, item := range items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO agent_capabilities(
				agent_id, kind, capability_id, name, description, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
				agentID, item.Kind, item.ID, item.Name, item.Description, unix(now)); err != nil {
				return agenttask.ErrInvalidInput
			}
		}
		return nil
	})
	return mapAgentWriteError(err)
}

func (s *AgentStore) ConversationCapabilities(ctx context.Context, userID, conversationID int64) ([]agenttask.Capability, error) {
	var allowed bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM conversation_members
		WHERE conversation_id = ? AND user_id = ?)`, conversationID, userID).Scan(&allowed); err != nil {
		return nil, err
	}
	if !allowed {
		return nil, agenttask.ErrNotFound
	}
	rows, err := s.db.QueryContext(ctx, `SELECT capability.kind, capability.capability_id,
		capability.name, capability.description
		FROM agent_capabilities capability
		JOIN agent_conversation_grants grant_row ON grant_row.agent_id = capability.agent_id
		WHERE grant_row.conversation_id = ?
		ORDER BY capability.kind, capability.name COLLATE NOCASE, capability.capability_id
		LIMIT ?`, conversationID, maxAgentCapabilities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]agenttask.Capability, 0)
	for rows.Next() {
		var item agenttask.Capability
		if err := rows.Scan(&item.Kind, &item.ID, &item.Name, &item.Description); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
