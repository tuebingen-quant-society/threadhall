package sqlite

import (
	"context"
	"database/sql"
)

func grantActiveAgentsToPublicChannel(ctx context.Context, tx *sql.Tx, conversationID, joinedAt int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_conversation_grants(
		agent_id, conversation_id, created_by, created_at)
		SELECT user_id, ?, created_by, ? FROM agents WHERE revoked_at IS NULL
		ON CONFLICT(agent_id, conversation_id) DO NOTHING`, conversationID, joinedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id, user_id, joined_at)
		SELECT ?, user_id, ? FROM agents WHERE revoked_at IS NULL
		ON CONFLICT(conversation_id, user_id) DO NOTHING`, conversationID, joinedAt)
	return err
}

func grantAgentToPublicChannels(ctx context.Context, tx *sql.Tx, agentID, createdBy, joinedAt int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO agent_conversation_grants(
		agent_id, conversation_id, created_by, created_at)
		SELECT ?, id, ?, ? FROM conversations WHERE kind = 'channel'
		ON CONFLICT(agent_id, conversation_id) DO NOTHING`, agentID, createdBy, joinedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id, user_id, joined_at)
		SELECT id, ?, ? FROM conversations WHERE kind = 'channel'
		ON CONFLICT(conversation_id, user_id) DO NOTHING`, agentID, joinedAt)
	return err
}
