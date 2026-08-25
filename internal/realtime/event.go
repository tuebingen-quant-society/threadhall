// Package realtime owns transport-neutral durable event contracts.
package realtime

import "encoding/json"

// Event is the compact durable event shared by HTTP results and later live
// transports. It deliberately contains no domain or transport-specific types.
type Event struct {
	Seq               int64              `json:"seq"`
	Type              string             `json:"type"`
	ConversationID    int64              `json:"conversation_id"`
	EntityID          int64              `json:"entity_id"`
	Payload           json.RawMessage    `json:"payload"`
	MembershipChanges []MembershipChange `json:"-"`
}

// MembershipChange is durable audience metadata consumed by the ordered hub.
// It is deliberately excluded from the stable wire envelope.
type MembershipChange struct {
	UserID int64
	Joined bool
}
