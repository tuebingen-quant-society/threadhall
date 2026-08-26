package message

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tuebingen-quant-society/threadhall/internal/agenttask"
)

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) (*Service, error) {
	if repository == nil || now == nil {
		return nil, fmt.Errorf("message repository and clock are required")
	}
	return &Service{repository: repository, now: now}, nil
}

func (s *Service) Send(ctx context.Context, command Send) (Result, error) {
	if command.ConversationID <= 0 || command.AuthorID <= 0 || !ValidBody(command.Body) ||
		!ValidIdempotencyKey(command.IdempotencyKey) || (command.ThreadRootID != nil && *command.ThreadRootID <= 0) ||
		(command.ReplyToMessageID != nil && *command.ReplyToMessageID <= 0) {
		return Result{}, ErrInvalidInput
	}
	rendered, err := RenderMarkdown(command.Body)
	if err != nil {
		return Result{}, fmt.Errorf("render message markdown: %w", err)
	}
	return s.repository.Send(ctx, SendRecord{
		ConversationID: command.ConversationID, AuthorID: command.AuthorID,
		ThreadRootID: command.ThreadRootID, ReplyToMessageID: command.ReplyToMessageID,
		Body: command.Body, RenderedBody: rendered, IdempotencyKey: command.IdempotencyKey,
		Mentions:  agenttask.MentionedAgents(command.Body),
		CreatedAt: s.now().UTC(),
	})
}

func (s *Service) Thread(ctx context.Context, query Thread) (ThreadPage, error) {
	if query.ConversationID <= 0 || query.RootMessageID <= 0 || query.UserID <= 0 || query.AfterID < 0 ||
		query.Limit < 0 || query.Limit > MaxPageLimit {
		return ThreadPage{}, ErrInvalidInput
	}
	if query.Limit == 0 {
		query.Limit = DefaultPageLimit
	}
	return s.repository.Thread(ctx, query)
}

func (s *Service) Threads(ctx context.Context, query ListThreads) (ThreadList, error) {
	if query.ConversationID <= 0 || query.UserID <= 0 || query.Limit < 0 || query.Limit > MaxPageLimit {
		return ThreadList{}, ErrInvalidInput
	}
	if query.Limit == 0 {
		query.Limit = DefaultPageLimit
	}
	return s.repository.Threads(ctx, query)
}

func (s *Service) MarkThreadRead(ctx context.Context, command MarkThreadRead) error {
	if command.UserID <= 0 || command.ConversationID <= 0 || command.RootMessageID <= 0 {
		return ErrInvalidInput
	}
	return s.repository.MarkThreadRead(ctx, command.UserID, command.ConversationID, command.RootMessageID, s.now().UTC())
}

func (s *Service) DeleteThread(ctx context.Context, command DeleteThread) error {
	if command.ActorID <= 0 || command.ConversationID <= 0 || command.RootMessageID <= 0 {
		return ErrInvalidInput
	}
	return s.repository.DeleteThread(ctx, command.ActorID, command.ConversationID, command.RootMessageID)
}

func (s *Service) RenameThread(ctx context.Context, command RenameThread) (ThreadRenameResult, error) {
	title := strings.TrimSpace(command.Title)
	if command.ActorID <= 0 || command.ConversationID <= 0 || command.RootMessageID <= 0 || title == "" ||
		!utf8.ValidString(title) || len(title) > 80 || !ValidIdempotencyKey(command.IdempotencyKey) {
		return ThreadRenameResult{}, ErrInvalidInput
	}
	return s.repository.RenameThread(ctx, command.ActorID, command.ConversationID, command.RootMessageID,
		title, command.IdempotencyKey, s.now().UTC())
}

func (s *Service) Edit(ctx context.Context, command Edit) (Result, error) {
	if command.MessageID <= 0 || command.AuthorID <= 0 || !ValidBody(command.Body) ||
		!ValidIdempotencyKey(command.IdempotencyKey) {
		return Result{}, ErrInvalidInput
	}
	rendered, err := RenderMarkdown(command.Body)
	if err != nil {
		return Result{}, fmt.Errorf("render message markdown: %w", err)
	}
	return s.repository.Edit(ctx, EditRecord{
		MessageID: command.MessageID, AuthorID: command.AuthorID, Body: command.Body,
		RenderedBody: rendered, IdempotencyKey: command.IdempotencyKey, EditedAt: s.now().UTC(),
	})
}

func (s *Service) Delete(ctx context.Context, command Delete) (Result, error) {
	if command.MessageID <= 0 || command.AuthorID <= 0 || !ValidIdempotencyKey(command.IdempotencyKey) {
		return Result{}, ErrInvalidInput
	}
	return s.repository.Delete(ctx, DeleteRecord{
		MessageID: command.MessageID, AuthorID: command.AuthorID,
		IdempotencyKey: command.IdempotencyKey, DeletedAt: s.now().UTC(),
	})
}

func (s *Service) History(ctx context.Context, query History) (Page, error) {
	if query.ConversationID <= 0 || query.UserID <= 0 || query.BeforeID < 0 ||
		query.Limit < 0 || query.Limit > MaxPageLimit {
		return Page{}, ErrInvalidInput
	}
	if query.Limit == 0 {
		query.Limit = DefaultPageLimit
	}
	return s.repository.History(ctx, query)
}

// ValidBody reports whether body satisfies the public raw-message contract.
func ValidBody(body string) bool {
	return body != "" && utf8.ValidString(body) && len(body) <= MaxBodyBytes && strings.TrimSpace(body) != ""
}

// ValidIdempotencyKey reports whether key is a bounded nonblank UTF-8 key.
func ValidIdempotencyKey(key string) bool {
	return strings.TrimSpace(key) != "" && utf8.ValidString(key) && len(key) <= MaxIdempotencyKeyBytes
}
