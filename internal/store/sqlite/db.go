// Package sqlite provides Threadhall's SQLite persistence foundation.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const maxReadConnections = 16

// Open opens, verifies, and migrates a SQLite database at path.
func Open(path string, readConnections int) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("SQLite path is required")
	}
	if readConnections <= 0 || readConnections > maxReadConnections {
		return nil, fmt.Errorf("read connections must be between 1 and %d", maxReadConnections)
	}

	dsnURL := url.URL{Scheme: "file", Path: path}
	query := dsnURL.Query()
	query.Set("_busy_timeout", "0")
	query.Set("_foreign_keys", "on")
	query.Set("_journal_mode", "WAL")
	query.Set("_synchronous", "FULL")
	dsnURL.RawQuery = query.Encode()

	db, err := sql.Open("sqlite3", dsnURL.String())
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(readConnections)
	db.SetMaxIdleConns(readConnections)

	if err := verifyCapabilities(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func verifyCapabilities(ctx context.Context, db *sql.DB) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("connect to SQLite: %w", err)
	}
	defer conn.Close()

	checks := []struct {
		name string
		want int
	}{
		{name: "busy_timeout", want: 0},
		{name: "foreign_keys", want: 1},
		{name: "synchronous", want: 2},
	}
	for _, check := range checks {
		var got int
		if err := conn.QueryRowContext(ctx, "PRAGMA "+check.name).Scan(&got); err != nil {
			return fmt.Errorf("read SQLite %s pragma: %w", check.name, err)
		}
		if got != check.want {
			return fmt.Errorf("SQLite %s pragma is %d, want %d", check.name, got, check.want)
		}
	}
	var journalMode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return fmt.Errorf("read SQLite journal_mode pragma: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("SQLite journal_mode is %q, want WAL", journalMode)
	}

	if _, err := conn.ExecContext(ctx, "CREATE VIRTUAL TABLE temp.threadhall_fts5_check USING fts5(content)"); err != nil {
		return fmt.Errorf("SQLite FTS5 support is required: %w", err)
	}
	defer conn.ExecContext(context.Background(), "DROP TABLE temp.threadhall_fts5_check")
	if _, err := conn.ExecContext(ctx, "INSERT INTO threadhall_fts5_check(content) VALUES ('threadhall capability')"); err != nil {
		return fmt.Errorf("verify SQLite FTS5 insert: %w", err)
	}
	var matches int
	if err := conn.QueryRowContext(ctx,
		"SELECT count(*) FROM threadhall_fts5_check WHERE threadhall_fts5_check MATCH 'capability'",
	).Scan(&matches); err != nil {
		return fmt.Errorf("verify SQLite FTS5 query: %w", err)
	}
	if matches != 1 {
		return fmt.Errorf("SQLite FTS5 verification returned %d matches, want 1", matches)
	}
	return nil
}
