// Package agenttask owns scoped agent invocation and durable task lifecycle.
package agenttask

import (
	"encoding/json"
	"strings"
	"time"
)

const (
	MaxContextMessages = 40
	MaxContextBytes    = 128 << 10
	MaxOutputBytes     = 64 << 10
	MaxInlineApps      = 4
	MaxInlineAppBytes  = 256 << 10
	MaxInlineAppsBytes = 512 << 10
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

type Capability struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CapabilityPage struct {
	Capabilities []Capability `json:"capabilities"`
}

type InlineApp struct {
	Server      string          `json:"server"`
	Tool        string          `json:"tool"`
	ResourceURI string          `json:"resource_uri"`
	HTML        string          `json:"html"`
	Arguments   json.RawMessage `json:"arguments"`
	Result      json.RawMessage `json:"result"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Question struct {
	ID       string           `json:"id"`
	Header   string           `json:"header"`
	Question string           `json:"question"`
	IsOther  bool             `json:"is_other"`
	Options  []QuestionOption `json:"options"`
}

func ValidQuestions(questions []Question) bool {
	if len(questions) > 3 {
		return false
	}
	for _, question := range questions {
		if strings.TrimSpace(question.ID) == "" || len(question.ID) > 64 || strings.TrimSpace(question.Header) == "" || len(question.Header) > 64 ||
			strings.TrimSpace(question.Question) == "" || len(question.Question) > 512 || len(question.Options) > 4 {
			return false
		}
		for _, option := range question.Options {
			if strings.TrimSpace(option.Label) == "" || len(option.Label) > 80 || len(option.Description) > 256 {
				return false
			}
		}
	}
	return true
}

func ValidInlineApps(apps []InlineApp) bool {
	if len(apps) > MaxInlineApps {
		return false
	}
	total := 0
	for _, app := range apps {
		total += len(app.HTML)
		if strings.TrimSpace(app.Server) == "" || len(app.Server) > 128 || strings.TrimSpace(app.Tool) == "" || len(app.Tool) > 128 ||
			!strings.HasPrefix(app.ResourceURI, "ui://") || len(app.ResourceURI) > 2048 || app.HTML == "" ||
			len(app.HTML) > MaxInlineAppBytes || total > MaxInlineAppsBytes || len(app.Arguments) > 64<<10 || len(app.Result) > 64<<10 ||
			(len(app.Arguments) > 0 && !json.Valid(app.Arguments)) || (len(app.Result) > 0 && !json.Valid(app.Result)) {
			return false
		}
	}
	return true
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
	TaskID, AgentID int64       `json:"-"`
	Output          string      `json:"output"`
	RuntimeThreadID string      `json:"runtime_thread_id"`
	Apps            []InlineApp `json:"apps,omitempty"`
	Questions       []Question  `json:"questions,omitempty"`
	CompletedAt     time.Time   `json:"-"`
}

type Failure struct {
	TaskID, AgentID int64
	Reason          string
	FailedAt        time.Time
}
