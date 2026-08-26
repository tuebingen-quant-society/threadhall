package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func (s *MessageStore) Threads(ctx context.Context, query message.ListThreads) (message.ThreadList, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT root.id, root.conversation_id, root.author_id,
		root.thread_root_id, root.body, root.rendered_body, root.created_at, root.edited_at, root.deleted_at,
		count(reply.id)
		FROM messages root
		JOIN conversation_members member ON member.conversation_id = root.conversation_id AND member.user_id = ?
		JOIN messages reply ON reply.thread_root_id = root.id
		WHERE root.conversation_id = ? AND root.reply_to_id IS NULL AND root.thread_root_id IS NULL
		GROUP BY root.id ORDER BY max(reply.id) DESC LIMIT ?`, query.UserID, query.ConversationID, query.Limit)
	if err != nil {
		return message.ThreadList{}, err
	}
	defer rows.Close()
	result := message.ThreadList{Threads: make([]message.ThreadSummary, 0, query.Limit)}
	for rows.Next() {
		var summary message.ThreadSummary
		var threadRoot, editedAt, deletedAt sql.NullInt64
		var createdAt int64
		if err := rows.Scan(&summary.Root.ID, &summary.Root.ConversationID, &summary.Root.AuthorID,
			&threadRoot, &summary.Root.Body, &summary.Root.RenderedBody, &createdAt, &editedAt, &deletedAt,
			&summary.ReplyCount); err != nil {
			return message.ThreadList{}, err
		}
		summary.Root.CreatedAt = time.Unix(createdAt, 0).UTC()
		if editedAt.Valid {
			value := time.Unix(editedAt.Int64, 0).UTC()
			summary.Root.EditedAt = &value
		}
		if deletedAt.Valid {
			value := time.Unix(deletedAt.Int64, 0).UTC()
			summary.Root.DeletedAt = &value
		}
		result.Threads = append(result.Threads, summary)
	}
	return result, rows.Err()
}
