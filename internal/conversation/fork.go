package conversation

import "time"

// ForkConversation creates a new named conversation from a source message or thread.
type ForkConversation struct {
	ActorID, SourceConversationID, SourceMessageID int64
	Kind                                           Kind
	Name, IdempotencyKey                           string
}

// Fork identifies the new conversation and its normalized source root.
type Fork struct {
	Conversation         Conversation `json:"conversation"`
	SourceConversationID int64        `json:"source_conversation_id"`
	SourceRootMessageID  int64        `json:"source_root_message_id"`
}

type ForkRecord struct {
	ActorID, SourceConversationID, SourceMessageID int64
	Kind                                           Kind
	Name, IdempotencyKey                           string
	CreatedAt                                      time.Time
}
