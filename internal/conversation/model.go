// Package conversation owns channels, direct messages, and membership policy.
package conversation

import "time"

// Kind is the persisted conversation visibility and shape.
type Kind string

const (
	KindChannel Kind = "channel"
	KindPrivate Kind = "private"
	KindDM      Kind = "dm"
)

const (
	DefaultPageLimit = 50
	MaxPageLimit     = 100
)

// Conversation is a membership-scoped channel or one-to-one direct message.
type Conversation struct {
	ID           int64     `json:"id"`
	Kind         Kind      `json:"kind"`
	Name         string    `json:"name,omitempty"`
	PeerUsername string    `json:"peer_username,omitempty"`
	CreatedBy    int64     `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// CreateChannel creates a public or private named channel.
type CreateChannel struct {
	CreatorID      int64
	Kind           Kind
	Name           string
	IdempotencyKey string
}

// CreateDM creates or returns the canonical direct message for two users.
type CreateDM struct {
	RequesterID    int64
	OtherUserID    int64
	IdempotencyKey string
}

// ListConversations is a descending-ID keyset page request.
type ListConversations struct {
	UserID   int64
	BeforeID int64
	Limit    int
}

// ConversationPage is a bounded keyset page.
type ConversationPage struct {
	Conversations []Conversation `json:"conversations"`
	NextBeforeID  int64          `json:"next_before_id,omitempty"`
}

// Member is a bounded public membership projection.
type Member struct {
	UserID        int64     `json:"user_id"`
	Username      string    `json:"username"`
	PrincipalKind string    `json:"principal_kind"`
	JoinedAt      time.Time `json:"joined_at"`
}

// ListMembers is a descending-user-ID keyset page request.
type ListMembers struct {
	UserID         int64
	ConversationID int64
	BeforeID       int64
	Limit          int
}

// MemberPage is a bounded keyset page.
type MemberPage struct {
	Members      []Member `json:"members"`
	NextBeforeID int64    `json:"next_before_id,omitempty"`
}

// ChangeMember applies the desired membership state to a named channel.
type ChangeMember struct {
	ActorID        int64
	ConversationID int64
	UserID         int64
	IdempotencyKey string
}
