package sqlite

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

// AgentStore persists distinct worker identities, grants, and task claims.
type AgentStore struct {
	db     *sql.DB
	writer *Writer
}

func NewAgentStore(db *sql.DB, writer *Writer) *AgentStore {
	return &AgentStore{db: db, writer: writer}
}

func (s *AgentStore) Create(ctx context.Context, command agenttask.CreateAgent) (agenttask.Agent, error) {
	var agent agenttask.Agent
	err := s.writer.Do(ctx, func(tx *sql.Tx) error {
		var admin bool
		if err := tx.QueryRowContext(ctx, `SELECT is_admin FROM users
			WHERE id = ? AND principal_kind = 'human'`, command.CreatedBy).Scan(&admin); err != nil || !admin {
			return agenttask.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO users(
			username, password_hash, is_admin, created_at, principal_kind)
			VALUES (?, '!', 0, ?, 'agent')`, command.Username, unix(command.CreatedAt))
		if err != nil {
			return agenttask.ErrConflict
		}
		agent.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO agents(user_id, token_hash, created_by, created_at)
			VALUES (?, ?, ?, ?)`, agent.ID, command.TokenHash[:], command.CreatedBy, unix(command.CreatedAt))
		if err != nil {
			return err
		}
		return grantAgentToPublicChannels(ctx, tx, agent.ID, command.CreatedBy, unix(command.CreatedAt))
	})
	if err != nil {
		return agenttask.Agent{}, mapAgentWriteError(err)
	}
	agent.Username, agent.CreatedAt = command.Username, command.CreatedAt.UTC()
	return agent, nil
}

func (s *AgentStore) Grant(ctx context.Context, command agenttask.Grant) error {
	err := s.writer.Do(ctx, func(tx *sql.Tx) error {
		if !isAdmin(ctx, tx, command.CreatedBy) {
			return agenttask.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO agent_conversation_grants(
			agent_id, conversation_id, created_by, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(agent_id, conversation_id) DO NOTHING`, command.AgentID,
			command.ConversationID, command.CreatedBy, unix(command.CreatedAt))
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			return nil
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO conversation_members(conversation_id, user_id, joined_at)
			VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, command.ConversationID, command.AgentID, unix(command.CreatedAt))
		return err
	})
	return mapAgentWriteError(err)
}

func (s *AgentStore) SetConversationPolicy(
	ctx context.Context,
	actorID, conversationID int64,
	policy agenttask.ConversationPolicy,
) error {
	if policy != agenttask.PolicyExplicit && policy != agenttask.PolicyHumanOnly {
		return agenttask.ErrInvalidInput
	}
	err := s.writer.Do(ctx, func(tx *sql.Tx) error {
		if !isAdmin(ctx, tx, actorID) {
			return agenttask.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `UPDATE conversations SET agent_policy = ? WHERE id = ?`, policy, conversationID)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return agenttask.ErrNotFound
		}
		if policy == agenttask.PolicyHumanOnly {
			_, err = tx.ExecContext(ctx, `UPDATE agent_tasks SET state = 'denied', completed_at = ?
				WHERE conversation_id = ? AND state = 'queued'`, time.Now().UTC().Unix(), conversationID)
		}
		return err
	})
	return mapAgentWriteError(err)
}

func (s *AgentStore) Claim(ctx context.Context, tokenHash [32]byte, now time.Time) (agenttask.Work, bool, error) {
	var work agenttask.Work
	err := s.writer.Do(ctx, func(tx *sql.Tx) error {
		agentID, err := authenticatedAgent(ctx, tx, tokenHash)
		if err != nil {
			return err
		}
		row := tx.QueryRowContext(ctx, `SELECT t.id, t.conversation_id, t.owner_id,
			t.invoking_message_id, t.thread_root_id, message.body, t.created_at
			FROM agent_tasks t JOIN messages message ON message.id = t.invoking_message_id
			JOIN conversations c ON c.id = t.conversation_id
			JOIN agent_conversation_grants grant_row ON grant_row.agent_id = t.agent_id
				AND grant_row.conversation_id = t.conversation_id
			WHERE t.agent_id = ? AND t.state = 'queued' AND c.agent_policy = 'explicit'
			ORDER BY t.id LIMIT 1`, agentID)
		var created int64
		var threadRoot sql.NullInt64
		work.Task.AgentID = agentID
		err = row.Scan(&work.Task.ID, &work.Task.ConversationID, &work.Task.OwnerID,
			&work.Task.InvokingMessageID, &threadRoot, &work.Task.InvocationBody, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		work.Task.State, work.Task.CreatedAt = agenttask.StateRunning, time.Unix(created, 0).UTC()
		if threadRoot.Valid {
			work.Task.ThreadRootID = &threadRoot.Int64
		}
		result, err := tx.ExecContext(ctx, `UPDATE agent_tasks SET state = 'running', started_at = ?
			WHERE id = ? AND state = 'queued'`, unix(now), work.Task.ID)
		if err != nil {
			return err
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return agenttask.ErrConflict
		}
		if err := createTaskMessage(ctx, tx, work.Task, now); err != nil {
			return err
		}
		work.Context, err = taskContext(ctx, tx, work.Task)
		work.Context = agenttask.BoundContext(work.Context)
		return err
	})
	if err != nil {
		return agenttask.Work{}, false, mapAgentWriteError(err)
	}
	if work.Task.ID == 0 {
		return agenttask.Work{}, false, nil
	}
	work.Prompt = agenttask.BuildPrompt(work.Task, work.Context)
	return work, true, nil
}

func authenticatedAgent(ctx context.Context, tx *sql.Tx, candidate [32]byte) (int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT user_id, token_hash FROM agents WHERE revoked_at IS NULL`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var stored []byte
		if err := rows.Scan(&id, &stored); err != nil {
			return 0, err
		}
		if len(stored) == len(candidate) && subtle.ConstantTimeCompare(stored, candidate[:]) == 1 {
			return id, nil
		}
	}
	return 0, agenttask.ErrUnauthenticated
}

func taskContext(ctx context.Context, tx *sql.Tx, task agenttask.Task) ([]agenttask.ContextMessage, error) {
	rows, err := tx.QueryContext(ctx, `SELECT m.id, u.username, m.body, m.created_at
		FROM messages m JOIN users u ON u.id = m.author_id
		WHERE m.conversation_id = ? AND m.id <= ? AND m.deleted_at IS NULL AND (
			(? IS NULL AND m.thread_root_id IS NULL) OR
			(? IS NOT NULL AND (m.id = ? OR m.thread_root_id = ?)))
		ORDER BY m.id DESC LIMIT ?`, task.ConversationID, task.InvokingMessageID,
		nullableInt64(task.ThreadRootID), nullableInt64(task.ThreadRootID),
		nullableInt64(task.ThreadRootID), nullableInt64(task.ThreadRootID), agenttask.MaxContextMessages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var reversed []agenttask.ContextMessage
	for rows.Next() {
		var item agenttask.ContextMessage
		var created int64
		if err := rows.Scan(&item.ID, &item.Author, &item.Body, &created); err != nil {
			return nil, err
		}
		item.CreatedAt = time.Unix(created, 0).UTC()
		reversed = append(reversed, item)
	}
	result := make([]agenttask.ContextMessage, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result, rows.Err()
}

func isAdmin(ctx context.Context, tx *sql.Tx, id int64) bool {
	var admin bool
	err := tx.QueryRowContext(ctx, `SELECT is_admin FROM users WHERE id = ? AND principal_kind = 'human'`, id).Scan(&admin)
	return err == nil && admin
}

func mapAgentWriteError(err error) error {
	if errors.Is(err, ErrBusy) {
		return agenttask.ErrBusy
	}
	return err
}
