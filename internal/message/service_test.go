package message

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceRejectsInvalidSendBeforePersistence(t *testing.T) {
	repository := &recordingRepository{}
	service, err := NewService(repository, func() time.Time {
		return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	invalid := []Send{
		{},
		{ConversationID: 1, AuthorID: 1, Body: "\xff", IdempotencyKey: "key"},
		{ConversationID: 1, AuthorID: 1, Body: " \n\t ", IdempotencyKey: "key"},
		{ConversationID: 1, AuthorID: 1, Body: string(make([]byte, MaxBodyBytes+1)), IdempotencyKey: "key"},
		{ConversationID: 1, AuthorID: 1, Body: "hello", IdempotencyKey: " "},
	}
	for _, command := range invalid {
		if _, err := service.Send(context.Background(), command); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Send(%#v) error = %v, want ErrInvalidInput", command, err)
		}
	}
	if repository.sendCalls != 0 {
		t.Fatalf("repository send calls = %d, want 0", repository.sendCalls)
	}
}

func TestServiceBuildsRenderedRecordsAndBoundsHistory(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.FixedZone("offset", 2*60*60))
	repository := &recordingRepository{}
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	body := "**hello**" + strings.Repeat("a", MaxBodyBytes-len("**hello**"))
	if _, err := service.Send(context.Background(), Send{
		ConversationID: 3, AuthorID: 4, Body: body, IdempotencyKey: "send-1",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if repository.sent.Body != body || !strings.Contains(repository.sent.RenderedBody, "<strong>hello</strong>") ||
		!repository.sent.CreatedAt.Equal(now.UTC()) {
		t.Fatalf("send record = %#v", repository.sent)
	}
	if _, err := service.Edit(context.Background(), Edit{
		MessageID: 8, AuthorID: 4, Body: "changed", IdempotencyKey: "edit-1",
	}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if repository.edited.MessageID != 8 || repository.edited.RenderedBody != "<p>changed</p>\n" {
		t.Fatalf("edit record = %#v", repository.edited)
	}
	if _, err := service.Delete(context.Background(), Delete{
		MessageID: 8, AuthorID: 4, IdempotencyKey: "delete-1",
	}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if repository.deleted.MessageID != 8 || !repository.deleted.DeletedAt.Equal(now.UTC()) {
		t.Fatalf("delete record = %#v", repository.deleted)
	}
	if _, err := service.History(context.Background(), History{ConversationID: 3, UserID: 4}); err != nil {
		t.Fatalf("History: %v", err)
	}
	if repository.history.Limit != DefaultPageLimit {
		t.Fatalf("history limit = %d, want %d", repository.history.Limit, DefaultPageLimit)
	}
	rootID := int64(7)
	if _, err := service.Send(context.Background(), Send{ConversationID: 3, AuthorID: 4, ThreadRootID: &rootID, Body: "reply", IdempotencyKey: "reply-1"}); err != nil {
		t.Fatalf("thread Send: %v", err)
	}
	if repository.sent.ThreadRootID == nil || *repository.sent.ThreadRootID != 7 {
		t.Fatalf("thread root = %#v", repository.sent.ThreadRootID)
	}
	if _, err := service.Thread(context.Background(), Thread{ConversationID: 3, RootMessageID: 7, UserID: 4}); err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if repository.thread.Limit != DefaultPageLimit {
		t.Fatalf("thread limit = %d", repository.thread.Limit)
	}
	if _, err := service.Threads(context.Background(), ListThreads{ConversationID: 3, UserID: 4}); err != nil || repository.threads.Limit != DefaultPageLimit {
		t.Fatalf("Threads = (%#v, %v)", repository.threads, err)
	}
}

type recordingRepository struct {
	sendCalls int
	sent      SendRecord
	edited    EditRecord
	deleted   DeleteRecord
	history   History
	thread    Thread
	threads   ListThreads
}

func (r *recordingRepository) Send(_ context.Context, record SendRecord) (Result, error) {
	r.sendCalls++
	r.sent = record
	return Result{}, nil
}

func (r *recordingRepository) Edit(_ context.Context, record EditRecord) (Result, error) {
	r.edited = record
	return Result{}, nil
}

func (r *recordingRepository) Delete(_ context.Context, record DeleteRecord) (Result, error) {
	r.deleted = record
	return Result{}, nil
}

func (r *recordingRepository) History(_ context.Context, query History) (Page, error) {
	r.history = query
	return Page{}, nil
}

func (r *recordingRepository) Thread(_ context.Context, query Thread) (ThreadPage, error) {
	r.thread = query
	return ThreadPage{}, nil
}

func (r *recordingRepository) Threads(_ context.Context, query ListThreads) (ThreadList, error) {
	r.threads = query
	return ThreadList{}, nil
}
