package message

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid message input")
	ErrNotFound     = errors.New("message resource not found")
	ErrConflict     = errors.New("message request conflicts with existing state")
	ErrForbidden    = errors.New("message operation forbidden")
	ErrBusy         = errors.New("message persistence is busy")
)

type SendRecord struct {
	ConversationID, AuthorID int64
	ThreadRootID             *int64
	ReplyToMessageID         *int64
	Body, RenderedBody       string
	IdempotencyKey           string
	Mentions                 []string
	CreatedAt                time.Time
}

type EditRecord struct {
	MessageID, AuthorID int64
	Body, RenderedBody  string
	IdempotencyKey      string
	EditedAt            time.Time
}

type DeleteRecord struct {
	MessageID, AuthorID int64
	IdempotencyKey      string
	DeletedAt           time.Time
}

// Repository is the storage-independent durable-message persistence port.
type Repository interface {
	Send(context.Context, SendRecord) (Result, error)
	Edit(context.Context, EditRecord) (Result, error)
	Delete(context.Context, DeleteRecord) (Result, error)
	History(context.Context, History) (Page, error)
	Thread(context.Context, Thread) (ThreadPage, error)
	Threads(context.Context, ListThreads) (ThreadList, error)
	MarkThreadRead(context.Context, int64, int64, int64, time.Time) error
	DeleteThread(context.Context, int64, int64, int64) error
}
