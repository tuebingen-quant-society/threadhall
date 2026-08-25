package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestWriterCloseDrainsAdmittedRequestsFIFO(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec("CREATE TABLE close_log (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)"); err != nil {
		t.Fatalf("create close log: %v", err)
	}
	writer, err := NewWriter(db, 2)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	results := make(chan error, 3)
	go func() {
		results <- writer.Do(context.Background(), func(tx *sql.Tx) error {
			close(started)
			<-release
			_, err := tx.Exec("INSERT INTO close_log(value) VALUES (1)")
			return err
		})
	}()
	<-started
	for _, value := range []int{2, 3} {
		value := value
		go func() { results <- writer.Do(context.Background(), insertCloseValue(value)) }()
		waitForQueuedRequests(t, writer, value-1)
	}

	closed := make(chan error, 1)
	go func() { closed <- writer.Close() }()
	waitForWriterClosed(t, writer)
	if err := writer.Do(context.Background(), insertCloseValue(4)); !errors.Is(err, errClosed) {
		t.Fatalf("Do after Close began = %v, want errClosed", err)
	}
	close(release)

	for range 3 {
		if err := <-results; err != nil {
			t.Fatalf("admitted Do: %v", err)
		}
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not drain admitted requests")
	}

	rows, err := db.Query("SELECT value FROM close_log ORDER BY id")
	if err != nil {
		t.Fatalf("query close log: %v", err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var value int
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan close log: %v", err)
		}
		got = append(got, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate close log: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("drained write order = %v, want [1 2 3]", got)
	}
}

func insertCloseValue(value int) WriteFunc {
	return func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO close_log(value) VALUES (?)", value)
		return err
	}
}

func waitForWriterClosed(t *testing.T, writer *Writer) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		writer.mu.Lock()
		closed := writer.closed
		writer.mu.Unlock()
		if closed {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("writer did not begin closing")
		}
		time.Sleep(time.Millisecond)
	}
}
