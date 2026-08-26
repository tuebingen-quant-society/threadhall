package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

const taskStartingMessage = "_Codex is starting…_"

func createTaskMessage(ctx context.Context, tx *sql.Tx, task agenttask.Task, now time.Time) error {
	rendered, err := message.RenderMarkdown(taskStartingMessage)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO messages(
		conversation_id, author_id, thread_root_id, body, rendered_body, idempotency_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, task.ConversationID, task.AgentID, nullableInt64(task.ThreadRootID),
		taskStartingMessage, rendered, fmt.Sprintf("agent-task-%d", task.ID), unix(now))
	if err != nil {
		return err
	}
	messageID, err := result.LastInsertId()
	if err != nil {
		return err
	}
	item := message.Message{ID: messageID, ConversationID: task.ConversationID, AuthorID: task.AgentID,
		ThreadRootID: task.ThreadRootID, Body: taskStartingMessage, RenderedBody: rendered, CreatedAt: now.UTC()}
	if _, err := insertMessageEvent(ctx, tx, "message.sent", item, now); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE agent_tasks SET output_message_id = ? WHERE id = ?`, messageID, task.ID)
	return err
}

func (s *AgentStore) Progress(ctx context.Context, tokenHash [32]byte, taskID int64, summary string, now time.Time) error {
	if taskID <= 0 || summary == "" || len(summary) > 4096 {
		return agenttask.ErrInvalidInput
	}
	return s.updateTaskMessage(ctx, tokenHash, taskID, summary, "", "", now)
}

func (s *AgentStore) Complete(ctx context.Context, tokenHash [32]byte, completion agenttask.Completion) error {
	if completion.TaskID <= 0 || completion.Output == "" || len(completion.Output) > agenttask.MaxOutputBytes ||
		len(completion.RuntimeThreadID) > 128 {
		return agenttask.ErrInvalidInput
	}
	return s.updateTaskMessage(ctx, tokenHash, completion.TaskID, completion.Output,
		completion.RuntimeThreadID, agenttask.StateCompleted, completion.CompletedAt)
}

func (s *AgentStore) Fail(ctx context.Context, tokenHash [32]byte, failure agenttask.Failure) error {
	body := "_Codex could not complete this task._"
	if failure.Reason == "interaction_unsupported" {
		body = "_Codex paused because interactive questions are not supported in this version._"
	} else if failure.Reason != "runtime_failed" {
		return agenttask.ErrInvalidInput
	}
	return s.updateTaskMessage(ctx, tokenHash, failure.TaskID, body, "", agenttask.StateFailed, failure.FailedAt)
}

func (s *AgentStore) updateTaskMessage(
	ctx context.Context,
	tokenHash [32]byte,
	taskID int64,
	body, runtimeThreadID string,
	finalState agenttask.State,
	now time.Time,
) error {
	rendered, err := message.RenderMarkdown(body)
	if err != nil {
		return err
	}
	err = s.writer.Do(ctx, func(tx *sql.Tx) error {
		agentID, err := authenticatedAgent(ctx, tx, tokenHash)
		if err != nil {
			return err
		}
		var item message.Message
		var threadRoot sql.NullInt64
		var created int64
		err = tx.QueryRowContext(ctx, `SELECT t.output_message_id, t.conversation_id, t.agent_id, m.thread_root_id, m.created_at
			FROM agent_tasks t JOIN messages m ON m.id = t.output_message_id
			JOIN conversations c ON c.id = t.conversation_id
			JOIN agent_conversation_grants grant_row ON grant_row.agent_id = t.agent_id
				AND grant_row.conversation_id = t.conversation_id
			WHERE t.id = ? AND t.agent_id = ? AND t.state = 'running'
				AND c.agent_policy = 'explicit'`, taskID, agentID).Scan(
			&item.ID, &item.ConversationID, &item.AuthorID, &threadRoot, &created,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return agenttask.ErrNotFound
		}
		if err != nil {
			return err
		}
		if threadRoot.Valid {
			item.ThreadRootID = &threadRoot.Int64
		}
		item.Body, item.RenderedBody = body, rendered
		item.CreatedAt = time.Unix(created, 0).UTC()
		edited := now.UTC()
		item.EditedAt = &edited
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET body = ?, rendered_body = ?, edited_at = ? WHERE id = ?`,
			body, rendered, unix(now), item.ID); err != nil {
			return err
		}
		if _, err := insertMessageEvent(ctx, tx, "message.edited", item, now); err != nil {
			return err
		}
		if finalState != "" {
			result, err := tx.ExecContext(ctx, `UPDATE agent_tasks SET state = ?, runtime_thread_id = ?, completed_at = ?
				WHERE id = ? AND state = 'running'`, finalState, runtimeThreadID, unix(now), taskID)
			if err != nil {
				return err
			}
			changed, _ := result.RowsAffected()
			if changed != 1 {
				return agenttask.ErrConflict
			}
		}
		return nil
	})
	return mapAgentWriteError(err)
}
