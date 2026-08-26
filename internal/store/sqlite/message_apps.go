package sqlite

import (
	"context"
	"database/sql"
	"strings"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
	"github.com/tuebingen-quant-society/threadhall/internal/message"
)

func replaceInlineApps(ctx context.Context, tx *sql.Tx, messageID int64, apps []agenttask.InlineApp) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_apps WHERE message_id = ?`, messageID); err != nil {
		return err
	}
	for ordinal, app := range apps {
		arguments, result := app.Arguments, app.Result
		if len(arguments) == 0 {
			arguments = []byte("null")
		}
		if len(result) == 0 {
			result = []byte("null")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO message_apps(
			message_id, ordinal, server, tool, resource_uri, html, arguments_json, result_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, messageID, ordinal, app.Server, app.Tool,
			app.ResourceURI, app.HTML, arguments, result); err != nil {
			return err
		}
	}
	return nil
}

func messageInlineApps(apps []agenttask.InlineApp) []message.InlineApp {
	result := make([]message.InlineApp, len(apps))
	for index, app := range apps {
		result[index] = message.InlineApp(app)
	}
	return result
}

func (s *MessageStore) attachInlineApps(ctx context.Context, items []message.Message) error {
	if len(items) == 0 {
		return nil
	}
	arguments := make([]any, len(items))
	placeholders := make([]string, len(items))
	byID := make(map[int64]int, len(items))
	for index := range items {
		arguments[index], placeholders[index], byID[items[index].ID] = items[index].ID, "?", index
	}
	rows, err := s.db.QueryContext(ctx, `SELECT message_id, server, tool, resource_uri, html,
		arguments_json, result_json FROM message_apps WHERE message_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY message_id, ordinal`, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var messageID int64
		var app message.InlineApp
		if err := rows.Scan(&messageID, &app.Server, &app.Tool, &app.ResourceURI, &app.HTML, &app.Arguments, &app.Result); err != nil {
			return err
		}
		if index, ok := byID[messageID]; ok {
			items[index].InlineApps = append(items[index].InlineApps, app)
		}
	}
	return rows.Err()
}
