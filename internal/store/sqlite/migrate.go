package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var migrations = []string{
	"migrations/001_core.sql",
	"migrations/002_conversations.sql",
	"migrations/003_messages.sql",
	"migrations/004_threads_forks.sql",
	"migrations/005_agents.sql",
	"migrations/006_agent_capabilities.sql",
	"migrations/007_message_apps.sql",
	"migrations/008_message_references.sql",
	"migrations/009_message_questions.sql",
	"migrations/010_profiles_reads.sql",
}

func migrate(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("read SQLite schema version: %w", err)
	}
	if current > len(migrations) {
		return fmt.Errorf("SQLite schema version %d is newer than supported version %d", current, len(migrations))
	}

	for index := current; index < len(migrations); index++ {
		version := index + 1
		script, err := migrationFiles.ReadFile(migrations[index])
		if err != nil {
			return fmt.Errorf("read migration %d: %w", version, err)
		}
		var prepare func(context.Context, *sql.Tx) error
		if version == 2 {
			prepare = renameCollidingV1Channels
		}
		if err := applyPreparedMigration(ctx, db, version, string(script), prepare); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, version int, script string) error {
	return applyPreparedMigration(ctx, db, version, script, nil)
}

func applyPreparedMigration(
	ctx context.Context,
	db *sql.DB,
	version int,
	script string,
	prepare func(context.Context, *sql.Tx) error,
) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", version, err)
	}
	defer tx.Rollback()

	if prepare != nil {
		if err := prepare(ctx, tx); err != nil {
			return fmt.Errorf("prepare migration %d: %w", version, err)
		}
	}
	if _, err := tx.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("apply migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}
