package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxChannelNameBytes = 80
	maxIdempotencyBytes = 128
	maxInitialMembers   = 100
)

// Service applies conversation validation before persistence.
type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository, now func() time.Time) (*Service, error) {
	if repository == nil || now == nil {
		return nil, fmt.Errorf("conversation repository and clock are required")
	}
	return &Service{repository: repository, now: now}, nil
}

func (s *Service) CreateChannel(ctx context.Context, command CreateChannel) (Conversation, error) {
	name := strings.TrimSpace(command.Name)
	if command.CreatorID <= 0 || (command.Kind != KindChannel && command.Kind != KindPrivate) ||
		name == "" || !utf8.ValidString(name) || len(name) > maxChannelNameBytes ||
		!validIdempotencyKey(command.IdempotencyKey) {
		return Conversation{}, ErrInvalidInput
	}
	members, ok := initialMembers(command.CreatorID, command.Kind, command.MemberIDs)
	if !ok {
		return Conversation{}, ErrInvalidInput
	}
	return s.repository.CreateChannel(ctx, ChannelRecord{
		CreatorID: command.CreatorID, Kind: command.Kind, Name: name,
		MemberIDs:      members,
		IdempotencyKey: command.IdempotencyKey, CreatedAt: s.now().UTC(),
	})
}

func initialMembers(creatorID int64, kind Kind, values []int64) ([]int64, bool) {
	if kind == KindChannel && len(values) > 0 || len(values) > maxInitialMembers {
		return nil, false
	}
	seen := map[int64]bool{creatorID: true}
	result := make([]int64, 0, len(values))
	for _, id := range values {
		if id <= 0 {
			return nil, false
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result, true
}

func (s *Service) Delete(ctx context.Context, command DeleteConversation) error {
	if command.ActorID <= 0 || command.ConversationID <= 0 {
		return ErrInvalidInput
	}
	return s.repository.DeleteConversation(ctx, command.ActorID, command.ConversationID)
}

func (s *Service) MarkRead(ctx context.Context, command MarkRead) error {
	if command.UserID <= 0 || command.ConversationID <= 0 {
		return ErrInvalidInput
	}
	return s.repository.MarkRead(ctx, command.UserID, command.ConversationID, s.now().UTC())
}

func (s *Service) CreateDM(ctx context.Context, command CreateDM) (Conversation, error) {
	if command.RequesterID <= 0 || command.OtherUserID <= 0 || command.RequesterID == command.OtherUserID ||
		!validIdempotencyKey(command.IdempotencyKey) {
		return Conversation{}, ErrInvalidInput
	}
	low, high := command.RequesterID, command.OtherUserID
	if low > high {
		low, high = high, low
	}
	return s.repository.CreateDM(ctx, DMRecord{
		RequesterID: command.RequesterID, OtherUserID: command.OtherUserID,
		UserLowID: low, UserHighID: high, IdempotencyKey: command.IdempotencyKey,
		CreatedAt: s.now().UTC(),
	})
}

func (s *Service) Fork(ctx context.Context, command ForkConversation) (Fork, error) {
	name := strings.TrimSpace(command.Name)
	if command.ActorID <= 0 || command.SourceConversationID <= 0 || command.SourceMessageID <= 0 ||
		(command.Kind != KindChannel && command.Kind != KindPrivate) || name == "" ||
		!utf8.ValidString(name) || len(name) > maxChannelNameBytes || !validIdempotencyKey(command.IdempotencyKey) {
		return Fork{}, ErrInvalidInput
	}
	return s.repository.Fork(ctx, ForkRecord{
		ActorID: command.ActorID, SourceConversationID: command.SourceConversationID,
		SourceMessageID: command.SourceMessageID, Kind: command.Kind, Name: name,
		IdempotencyKey: command.IdempotencyKey, CreatedAt: s.now().UTC(),
	})
}

func (s *Service) List(ctx context.Context, query ListConversations) (ConversationPage, error) {
	limit, err := pageLimit(query.UserID, query.BeforeID, query.Limit)
	if err != nil {
		return ConversationPage{}, err
	}
	return s.repository.List(ctx, query.UserID, query.BeforeID, limit)
}

func (s *Service) Detail(ctx context.Context, userID, conversationID int64) (Conversation, error) {
	if userID <= 0 || conversationID <= 0 {
		return Conversation{}, ErrInvalidInput
	}
	return s.repository.Detail(ctx, userID, conversationID)
}

func (s *Service) Members(ctx context.Context, query ListMembers) (MemberPage, error) {
	limit, err := pageLimit(query.UserID, query.BeforeID, query.Limit)
	if err != nil || query.ConversationID <= 0 {
		return MemberPage{}, ErrInvalidInput
	}
	return s.repository.ListMembers(ctx, query.UserID, query.ConversationID, query.BeforeID, limit)
}

// CanRead is the mandatory authorization seam for history and event consumers.
func (s *Service) CanRead(ctx context.Context, userID, conversationID int64) (bool, error) {
	if userID <= 0 || conversationID <= 0 {
		return false, ErrInvalidInput
	}
	return s.repository.CanRead(ctx, userID, conversationID)
}

func (s *Service) AddMember(ctx context.Context, command ChangeMember) error {
	record, err := s.memberRecord(command)
	if err != nil {
		return err
	}
	return s.repository.AddMember(ctx, record)
}

func (s *Service) RemoveMember(ctx context.Context, command ChangeMember) error {
	record, err := s.memberRecord(command)
	if err != nil {
		return err
	}
	return s.repository.RemoveMember(ctx, record)
}

func (s *Service) memberRecord(command ChangeMember) (MemberRecord, error) {
	if command.ActorID <= 0 || command.ConversationID <= 0 || command.UserID <= 0 ||
		!validIdempotencyKey(command.IdempotencyKey) {
		return MemberRecord{}, ErrInvalidInput
	}
	return MemberRecord{
		ActorID: command.ActorID, ConversationID: command.ConversationID, UserID: command.UserID,
		IdempotencyKey: command.IdempotencyKey, ChangedAt: s.now().UTC(),
	}, nil
}

func pageLimit(userID, beforeID int64, limit int) (int, error) {
	if userID <= 0 || beforeID < 0 || limit < 0 || limit > MaxPageLimit {
		return 0, ErrInvalidInput
	}
	if limit == 0 {
		limit = DefaultPageLimit
	}
	return limit, nil
}

func validIdempotencyKey(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value) && len(value) <= maxIdempotencyBytes
}
