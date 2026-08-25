package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestWriterCancellationRollsBackAndCloseDrainsNextWrite(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE cancel_log (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create cancel log: %v", err)
	}
	writer, err := NewWriter(db, 1)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	inserted := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	canceledResult := make(chan error, 1)
	go func() {
		canceledResult <- writer.Do(ctx, func(tx *sql.Tx) error {
			if _, err := tx.Exec("INSERT INTO cancel_log(value) VALUES (1)"); err != nil {
				return err
			}
			close(inserted)
			<-release
			return nil
		})
	}()
	<-inserted

	nextResult := make(chan error, 1)
	go func() {
		nextResult <- writer.Do(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec("INSERT INTO cancel_log(value) VALUES (2)")
			return err
		})
	}()
	waitForQueuedRequests(t, writer, 1)
	cancel()
	select {
	case err := <-canceledResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Do = %v, want context.Canceled", err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("canceled Do blocked")
	}

	closed := make(chan error, 1)
	go func() { closed <- writer.Close() }()
	waitForWriterClosed(t, writer)
	close(release)
	select {
	case err := <-nextResult:
		if err != nil {
			t.Fatalf("write queued behind cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued write did not make progress")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close deadlocked with cancellation")
	}

	rows, err := db.Query("SELECT value FROM cancel_log ORDER BY id")
	if err != nil {
		t.Fatalf("query cancel log: %v", err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan cancel log: %v", err)
		}
		got = append(got, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate cancel log: %v", err)
	}
	if len(got) != 1 || got[0] != 2 {
		t.Fatalf("persisted values = %v, want [2]", got)
	}
}
