package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenConfiguresEveryConnection(t *testing.T) {
	const connectionLimit = 3
	db, err := Open(filepath.Join(t.TempDir(), "threadhall.db"), connectionLimit)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	if got := db.Stats().MaxOpenConnections; got != connectionLimit {
		t.Fatalf("maximum open connections = %d, want %d", got, connectionLimit)
	}

	ctx := context.Background()
	connections := make([]*sql.Conn, 0, connectionLimit)
	for range connectionLimit {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("acquire connection: %v", err)
		}
		connections = append(connections, conn)
		defer conn.Close()
	}
	if got := db.Stats().OpenConnections; got != connectionLimit {
		t.Fatalf("open connections = %d, want %d", got, connectionLimit)
	}

	for index, conn := range connections {
		assertConnPragmaInt(t, conn, "foreign_keys", 1, index)
		assertConnPragmaText(t, conn, "journal_mode", "wal", index)
		assertConnPragmaInt(t, conn, "synchronous", 2, index)
		assertConnPragmaInt(t, conn, "busy_timeout", 0, index)
	}

	conn := connections[0]
	if _, err := conn.ExecContext(ctx, "CREATE VIRTUAL TABLE temp.fts5_check USING fts5(content)"); err != nil {
		t.Fatalf("FTS5 is unavailable: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO fts5_check(content) VALUES ('bounded sqlite writes')"); err != nil {
		t.Fatalf("insert FTS5 check row: %v", err)
	}
	var matches int
	if err := conn.QueryRowContext(ctx,
		"SELECT count(*) FROM fts5_check WHERE fts5_check MATCH 'sqlite'",
	).Scan(&matches); err != nil {
		t.Fatalf("query FTS5 check row: %v", err)
	}
	if matches != 1 {
		t.Fatalf("FTS5 matches = %d, want 1", matches)
	}
}

func assertConnPragmaInt(t *testing.T, conn *sql.Conn, name string, want, index int) {
	t.Helper()
	var got int
	if err := conn.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&got); err != nil {
		t.Fatalf("connection %d PRAGMA %s: %v", index, name, err)
	}
	if got != want {
		t.Fatalf("connection %d PRAGMA %s = %d, want %d", index, name, got, want)
	}
}

func assertConnPragmaText(t *testing.T, conn *sql.Conn, name, want string, index int) {
	t.Helper()
	var got string
	if err := conn.QueryRowContext(context.Background(), "PRAGMA "+name).Scan(&got); err != nil {
		t.Fatalf("connection %d PRAGMA %s: %v", index, name, err)
	}
	if !strings.EqualFold(got, want) {
		t.Fatalf("connection %d PRAGMA %s = %q, want %q", index, name, got, want)
	}
}
