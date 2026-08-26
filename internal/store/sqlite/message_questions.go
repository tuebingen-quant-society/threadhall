package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func replaceQuestions(ctx context.Context, tx *sql.Tx, messageID int64, questions []agenttask.Question) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_questions WHERE message_id = ?`, messageID); err != nil {
		return err
	}
	if len(questions) == 0 {
		return nil
	}
	encoded, err := json.Marshal(questions)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO message_questions(message_id, questions_json) VALUES (?, ?)`, messageID, encoded)
	return err
}

func (s *MessageStore) attachQuestions(ctx context.Context, items []message.Message) error {
	if len(items) == 0 {
		return nil
	}
	arguments := make([]any, len(items))
	placeholders := make([]string, len(items))
	byID := make(map[int64]int, len(items))
	for index := range items {
		arguments[index], placeholders[index], byID[items[index].ID] = items[index].ID, "?", index
	}
	rows, err := s.db.QueryContext(ctx, `SELECT message_id, questions_json FROM message_questions
		WHERE message_id IN (`+strings.Join(placeholders, ",")+`)`, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID int64
		var encoded []byte
		if err := rows.Scan(&messageID, &encoded); err != nil {
			return err
		}
		index, ok := byID[messageID]
		if !ok || json.Unmarshal(encoded, &items[index].Questions) != nil || !agenttask.ValidQuestions(items[index].Questions) {
			return agenttask.ErrInvalidInput
		}
	}
	return rows.Err()
}
