package conversation

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid conversation input")
	ErrNotFound     = errors.New("conversation not found")
	ErrConflict     = errors.New("conversation conflict")
	ErrForbidden    = errors.New("conversation membership administration forbidden")
	ErrBusy         = errors.New("conversation persistence is busy")
)

// ChannelRecord is the validated named-channel persistence command.
type ChannelRecord struct {
	CreatorID      int64
	Kind           Kind
	Name           string
	MemberIDs      []int64
	IdempotencyKey string
	CreatedAt      time.Time
}

// DMRecord is a validated direct-message persistence command.
type DMRecord struct {
	RequesterID, OtherUserID int64
	UserLowID, UserHighID    int64
	IdempotencyKey           string
	CreatedAt                time.Time
}

// MemberRecord is a validated membership mutation.
type MemberRecord struct {
	ActorID, ConversationID, UserID int64
	IdempotencyKey                  string
	ChangedAt                       time.Time
}

type RenameRecord struct {
	ActorID, ConversationID int64
	Name, IdempotencyKey    string
	RenamedAt               time.Time
}

// Repository is the storage-independent conversation persistence port.
type Repository interface {
	CreateChannel(context.Context, ChannelRecord) (Conversation, error)
	CreateDM(context.Context, DMRecord) (Conversation, error)
	Fork(context.Context, ForkRecord) (Fork, error)
	List(context.Context, int64, int64, int) (ConversationPage, error)
	Detail(context.Context, int64, int64) (Conversation, error)
	ListMembers(context.Context, int64, int64, int64, int) (MemberPage, error)
	CanRead(context.Context, int64, int64) (bool, error)
	AddMember(context.Context, MemberRecord) error
	RemoveMember(context.Context, MemberRecord) error
	DeleteConversation(context.Context, int64, int64) error
	RenameConversation(context.Context, RenameRecord) (Conversation, error)
	MarkRead(context.Context, int64, int64, time.Time) error
}
