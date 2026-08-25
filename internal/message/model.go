// Package message owns durable text messages and membership-filtered history.
package message

import (
	"time"

	"github.com/tuebingen-quant-society/threadhall/internal/realtime"
)

const (
	MaxBodyBytes           = 16 << 10
	MaxIdempotencyKeyBytes = 128
	DefaultPageLimit       = 50
	MaxPageLimit           = 100
)

// Message is a durable root text message. Deleted messages retain identity and
// ordering while exposing empty body fields.
type Message struct {
	ID             int64      `json:"id"`
	ConversationID int64      `json:"conversation_id"`
	AuthorID       int64      `json:"author_id"`
	Body           string     `json:"body"`
	RenderedBody   string     `json:"rendered_body"`
	CreatedAt      time.Time  `json:"created_at"`
	EditedAt       *time.Time `json:"edited_at,omitempty"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
}

type Send struct {
	ConversationID, AuthorID int64
	Body, IdempotencyKey     string
}

type Edit struct {
	MessageID, AuthorID  int64
	Body, IdempotencyKey string
}

type Delete struct {
	MessageID, AuthorID int64
	IdempotencyKey      string
}

type History struct {
	ConversationID, UserID int64
	BeforeID               int64
	Limit                  int
}

type Page struct {
	Messages     []Message `json:"messages"`
	NextBeforeID int64     `json:"next_before_id,omitempty"`
}

type Result struct {
	Message Message        `json:"message"`
	Event   realtime.Event `json:"event"`
}
