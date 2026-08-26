package conversation

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestServiceCreatesTrimmedNamedChannel(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	repository := &recordingRepository{
		created: Conversation{ID: 9, Kind: KindChannel, Name: "general", CreatedBy: 4, CreatedAt: now},
	}
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	created, err := service.CreateChannel(context.Background(), CreateChannel{
		CreatorID: 4, Kind: KindChannel, Name: "  general  ", IdempotencyKey: "create-general",
	})
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if created.ID != 9 || repository.channel.Name != "general" || repository.channel.CreatedAt != now {
		t.Fatalf("created/record = (%#v, %#v)", created, repository.channel)
	}
}

func TestServiceRejectsUnboundedOrMalformedChannelInputs(t *testing.T) {
	service, err := NewService(&recordingRepository{}, time.Now)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	for _, command := range []CreateChannel{
		{CreatorID: 1, Kind: KindChannel, Name: " ", IdempotencyKey: "key"},
		{CreatorID: 1, Kind: KindChannel, Name: strings.Repeat("x", maxChannelNameBytes+1), IdempotencyKey: "key"},
		{CreatorID: 1, Kind: KindChannel, Name: string([]byte{0xff}), IdempotencyKey: "key"},
		{CreatorID: 1, Kind: KindDM, Name: "named-dm", IdempotencyKey: "key"},
		{CreatorID: 1, Kind: KindPrivate, Name: "staff", IdempotencyKey: " "},
		{CreatorID: 1, Kind: KindPrivate, Name: "staff", IdempotencyKey: strings.Repeat("k", maxIdempotencyBytes+1)},
	} {
		if _, err := service.CreateChannel(context.Background(), command); err != ErrInvalidInput {
			t.Fatalf("CreateChannel(%#v) error = %v, want ErrInvalidInput", command, err)
		}
	}
}

func TestServiceCanonicalizesDistinctDirectMessagePair(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	repository := &recordingRepository{
		created: Conversation{ID: 11, Kind: KindDM, CreatedBy: 9, CreatedAt: now},
	}
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	created, err := service.CreateDM(context.Background(), CreateDM{
		RequesterID: 9, OtherUserID: 4, IdempotencyKey: "dm-4-9",
	})
	if err != nil {
		t.Fatalf("CreateDM: %v", err)
	}
	if created.ID != 11 || repository.dm.UserLowID != 4 || repository.dm.UserHighID != 9 {
		t.Fatalf("created/record = (%#v, %#v)", created, repository.dm)
	}

	_, err = service.CreateDM(context.Background(), CreateDM{
		RequesterID: 9, OtherUserID: 9, IdempotencyKey: "self-dm",
	})
	if err != ErrInvalidInput {
		t.Fatalf("self CreateDM error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceBoundsConversationPages(t *testing.T) {
	repository := &recordingRepository{}
	service, err := NewService(repository, time.Now)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := service.List(context.Background(), ListConversations{UserID: 4, Limit: MaxPageLimit + 1}); err != ErrInvalidInput {
		t.Fatalf("oversized List error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.List(context.Background(), ListConversations{UserID: 4}); err != nil {
		t.Fatalf("default List: %v", err)
	}
	if repository.listLimit != DefaultPageLimit {
		t.Fatalf("repository limit = %d, want %d", repository.listLimit, DefaultPageLimit)
	}
}

func TestServiceScopesConversationReadsToRequester(t *testing.T) {
	repository := &recordingRepository{created: Conversation{ID: 9, Kind: KindPrivate}}
	service, err := NewService(repository, time.Now)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := service.Detail(context.Background(), 4, 9); err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if _, err := service.Members(context.Background(), ListMembers{UserID: 4, ConversationID: 9}); err != nil {
		t.Fatalf("Members: %v", err)
	}
	allowed, err := service.CanRead(context.Background(), 4, 9)
	if err != nil || !allowed {
		t.Fatalf("CanRead = (%t, %v)", allowed, err)
	}
	if repository.readUserID != 4 || repository.readConversationID != 9 || repository.memberLimit != DefaultPageLimit {
		t.Fatalf("read scope = user %d conversation %d member limit %d", repository.readUserID, repository.readConversationID, repository.memberLimit)
	}
}

func TestServiceValidatesMembershipMutations(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	repository := &recordingRepository{}
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	command := ChangeMember{ActorID: 1, ConversationID: 9, UserID: 4, IdempotencyKey: "member-4"}
	if err := service.AddMember(context.Background(), command); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	if repository.member.ActorID != 1 || repository.member.ChangedAt != now {
		t.Fatalf("membership record = %#v", repository.member)
	}
	command.IdempotencyKey = ""
	if err := service.RemoveMember(context.Background(), command); err != ErrInvalidInput {
		t.Fatalf("missing-key RemoveMember error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceValidatesConversationFork(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	repository := &recordingRepository{}
	service, err := NewService(repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	_, err = service.Fork(context.Background(), ForkConversation{
		ActorID: 1, SourceConversationID: 4, SourceMessageID: 9,
		Kind: KindPrivate, Name: "  investigation  ", IdempotencyKey: "fork-1",
	})
	if err != nil || repository.fork.Name != "investigation" || repository.fork.CreatedAt != now {
		t.Fatalf("Fork = (%#v, %v)", repository.fork, err)
	}
	if _, err := service.Fork(context.Background(), ForkConversation{}); err != ErrInvalidInput {
		t.Fatalf("invalid Fork error = %v", err)
	}
}

type recordingRepository struct {
	channel                        ChannelRecord
	dm                             DMRecord
	created                        Conversation
	listLimit                      int
	readUserID, readConversationID int64
	memberLimit                    int
	member                         MemberRecord
	fork                           ForkRecord
}

func (r *recordingRepository) CreateChannel(_ context.Context, record ChannelRecord) (Conversation, error) {
	r.channel = record
	return r.created, nil
}

func (r *recordingRepository) CreateDM(_ context.Context, record DMRecord) (Conversation, error) {
	r.dm = record
	return r.created, nil
}

func (r *recordingRepository) Fork(_ context.Context, record ForkRecord) (Fork, error) {
	r.fork = record
	return Fork{}, nil
}

func (r *recordingRepository) List(_ context.Context, _ int64, _ int64, limit int) (ConversationPage, error) {
	r.listLimit = limit
	return ConversationPage{}, nil
}

func (r *recordingRepository) Detail(_ context.Context, userID, conversationID int64) (Conversation, error) {
	r.readUserID, r.readConversationID = userID, conversationID
	return r.created, nil
}

func (r *recordingRepository) ListMembers(_ context.Context, userID, conversationID, _ int64, limit int) (MemberPage, error) {
	r.readUserID, r.readConversationID, r.memberLimit = userID, conversationID, limit
	return MemberPage{}, nil
}

func (r *recordingRepository) CanRead(_ context.Context, userID, conversationID int64) (bool, error) {
	r.readUserID, r.readConversationID = userID, conversationID
	return true, nil
}

func (r *recordingRepository) AddMember(_ context.Context, record MemberRecord) error {
	r.member = record
	return nil
}

func (r *recordingRepository) RemoveMember(_ context.Context, record MemberRecord) error {
	r.member = record
	return nil
}
