package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "threadhall.db"), 2)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return db
}

func TestOpenConfiguresSQLite(t *testing.T) {
	db := openTestDB(t)

	assertPragmaInt(t, db, "foreign_keys", 1)
	assertPragmaText(t, db, "journal_mode", "wal")
	assertPragmaInt(t, db, "synchronous", 2)

	if _, err := db.Exec("CREATE VIRTUAL TABLE temp.fts5_check USING fts5(content)"); err != nil {
		t.Fatalf("FTS5 is unavailable: %v", err)
	}
	if _, err := db.Exec("INSERT INTO fts5_check(content) VALUES ('bounded sqlite writes')"); err != nil {
		t.Fatalf("insert FTS5 check row: %v", err)
	}
	var matches int
	if err := db.QueryRow("SELECT count(*) FROM fts5_check WHERE fts5_check MATCH 'sqlite'").Scan(&matches); err != nil {
		t.Fatalf("query FTS5 check row: %v", err)
	}
	if matches != 1 {
		t.Fatalf("FTS5 matches = %d, want 1", matches)
	}
}

func TestStorageLimitsArePositiveAndBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "threadhall.db")
	for _, connections := range []int{0, maxReadConnections + 1} {
		if db, err := Open(path, connections); err == nil {
			_ = db.Close()
			t.Errorf("Open connections=%d error = nil, want validation error", connections)
		}
	}

	db := openTestDB(t)
	for _, queueSize := range []int{0, maxWriterQueueSize + 1} {
		if writer, err := NewWriter(db, queueSize); err == nil {
			_ = writer.Close()
			t.Errorf("NewWriter queueSize=%d error = nil, want validation error", queueSize)
		}
	}
}

func TestWriterPreservesFIFOAndRejectsSaturation(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE write_log (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create write log: %v", err)
	}
	writer, err := NewWriter(db, 2)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})

	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan error, 3)
	go func() {
		results <- writer.Do(context.Background(), func(tx *sql.Tx) error {
			close(started)
			<-release
			_, err := tx.Exec("INSERT INTO write_log(value) VALUES (1)")
			return err
		})
	}()
	<-started

	for _, value := range []int{2, 3} {
		value := value
		go func() {
			results <- writer.Do(context.Background(), insertLogValue(value))
		}()
		waitForQueuedRequests(t, writer, value-1)
	}

	busy := make(chan error, 1)
	go func() { busy <- writer.Do(context.Background(), insertLogValue(4)) }()
	select {
	case err := <-busy:
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("saturated Do() error = %v, want ErrBusy", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("saturated Do() blocked")
	}

	close(release)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatalf("Do: %v", err)
		}
	}

	rows, err := db.Query("SELECT value FROM write_log ORDER BY id")
	if err != nil {
		t.Fatalf("query write log: %v", err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan write log: %v", err)
		}
		got = append(got, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate write log: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("write order = %v, want [1 2 3]", got)
	}
}

func TestWriterCancellationDoesNotBlockWorker(t *testing.T) {
	db := openTestDB(t)
	writer, err := NewWriter(db, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- writer.Do(ctx, func(*sql.Tx) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Do() error = %v, want context.Canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("canceled Do() blocked")
	}

	close(release)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestWriterRollsBackFailedWrite(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE write_log (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("create write log: %v", err)
	}
	writer, err := NewWriter(db, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	t.Cleanup(func() {
		if err := writer.Close(); err != nil {
			t.Errorf("close writer: %v", err)
		}
	})

	wantErr := errors.New("reject write")
	err = writer.Do(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO write_log DEFAULT VALUES"); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Do() error = %v, want %v", err, wantErr)
	}

	var count int
	if err := db.QueryRow("SELECT count(*) FROM write_log").Scan(&count); err != nil {
		t.Fatalf("count rolled-back writes: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back write count = %d, want 0", count)
	}
}

func insertLogValue(value int) WriteFunc {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO write_log(value) VALUES (?)", value)
		return err
	}
}

func waitForQueuedRequests(t *testing.T, writer *Writer, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for len(writer.requests) != want {
		if time.Now().After(deadline) {
			t.Fatalf("queued requests = %d, want %d", len(writer.requests), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func assertPragmaInt(t *testing.T, db *sql.DB, name string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("read PRAGMA %s: %v", name, err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %d, want %d", name, got, want)
	}
}

func assertPragmaText(t *testing.T, db *sql.DB, name, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow("PRAGMA " + name).Scan(&got); err != nil {
		t.Fatalf("read PRAGMA %s: %v", name, err)
	}
	if got != want {
		t.Fatalf("PRAGMA %s = %q, want %q", name, got, want)
	}
}
