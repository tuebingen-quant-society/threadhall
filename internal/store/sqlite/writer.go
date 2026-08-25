package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

const maxWriterQueueSize = 4096

var (
	// ErrBusy reports that the bounded writer queue is saturated.
	ErrBusy   = errors.New("sqlite writer is busy")
	errClosed = errors.New("sqlite writer is closed")
)

// WriteFunc performs one mutation inside the writer-owned transaction.
type WriteFunc func(*sql.Tx) error

type request struct {
	ctx    context.Context
	write  WriteFunc
	result chan error
}

// Writer serializes SQLite mutations through a bounded FIFO queue.
type Writer struct {
	db       *sql.DB
	requests chan request
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
}

// NewWriter starts a single bounded SQLite writer.
func NewWriter(db *sql.DB, queueSize int) (*Writer, error) {
	if db == nil {
		return nil, errors.New("SQLite database is required")
	}
	if queueSize <= 0 || queueSize > maxWriterQueueSize {
		return nil, fmt.Errorf("writer queue size must be between 1 and %d", maxWriterQueueSize)
	}
	writer := &Writer{
		db:       db,
		requests: make(chan request, queueSize),
		done:     make(chan struct{}),
	}
	go writer.run()
	return writer, nil
}

// Do submits fn without waiting for queue capacity and waits for its result.
func (w *Writer) Do(ctx context.Context, fn WriteFunc) error {
	if ctx == nil {
		return errors.New("write context is required")
	}
	if fn == nil {
		return errors.New("write function is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	req := request{ctx: ctx, write: fn, result: make(chan error, 1)}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errClosed
	}
	select {
	case w.requests <- req:
		w.mu.Unlock()
	default:
		w.mu.Unlock()
		return ErrBusy
	}

	select {
	case err := <-req.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close stops admissions, drains admitted requests FIFO, and waits for them.
func (w *Writer) Close() error {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		close(w.requests)
	}
	w.mu.Unlock()
	<-w.done
	return nil
}

func (w *Writer) run() {
	defer close(w.done)
	for req := range w.requests {
		req.result <- w.execute(req)
	}
}

func (w *Writer) execute(req request) error {
	if err := req.ctx.Err(); err != nil {
		return err
	}
	conn, err := w.db.Conn(req.ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := req.ctx.Err(); err != nil {
		return err
	}

	// The request context controls connection acquisition and admission. The
	// transaction itself is rolled back synchronously below so the next FIFO
	// write cannot race database/sql's asynchronous cancellation rollback.
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := req.write(tx); err != nil {
		return err
	}
	if err := req.ctx.Err(); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return errors.Join(err, rollbackErr)
		}
		return err
	}
	return tx.Commit()
}
