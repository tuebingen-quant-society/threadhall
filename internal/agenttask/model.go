// Package agenttask owns scoped agent invocation and durable task lifecycle.
package agenttask

import "time"

const (
	MaxContextMessages = 40
	MaxContextBytes    = 128 << 10
	MaxOutputBytes     = 64 << 10
)

type ConversationPolicy string

const (
	PolicyExplicit  ConversationPolicy = "explicit"
	PolicyHumanOnly ConversationPolicy = "human_only"
)

type Agent struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateAgent struct {
	Username  string
	TokenHash [32]byte
	CreatedBy int64
	CreatedAt time.Time
}

type Grant struct {
	AgentID, ConversationID, CreatedBy int64
	CreatedAt                          time.Time
}

type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

// Task is the durable bridge between a Threadhall request and one Codex thread.
type Task struct {
	ID                int64     `json:"id"`
	AgentID           int64     `json:"agent_id"`
	ConversationID    int64     `json:"conversation_id"`
	OwnerID           int64     `json:"owner_id"`
	InvokingMessageID int64     `json:"invoking_message_id"`
	ThreadRootID      *int64    `json:"thread_root_id,omitempty"`
	InvocationBody    string    `json:"invocation_body"`
	State             State     `json:"state"`
	RuntimeThreadID   string    `json:"runtime_thread_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

type ContextMessage struct {
	ID        int64     `json:"id"`
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Work struct {
	Task    Task             `json:"task"`
	Context []ContextMessage `json:"context"`
	Prompt  string           `json:"prompt"`
}

type Completion struct {
	TaskID, AgentID int64
	Output          string
	RuntimeThreadID string
	CompletedAt     time.Time
}

type Failure struct {
	TaskID, AgentID int64
	Reason          string
	FailedAt        time.Time
}
