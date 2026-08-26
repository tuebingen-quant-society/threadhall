package agentprotocol

import (
	"encoding/json"
	"fmt"
)

type Envelope struct {
	Version uint16          `json:"version"`
	Seq     uint64          `json:"seq"`
	Type    Type            `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func (e Envelope) String() string {
	return fmt.Sprintf("agentprotocol.Envelope{version:%d seq:%d type:%q payload:[redacted]}", e.Version, e.Seq, e.Type)
}

type Negotiate struct {
	Minimum      uint16   `json:"minimum"`
	Maximum      uint16   `json:"maximum"`
	Capabilities []string `json:"capabilities"`
}

type Authenticate struct {
	WorkerID string `json:"worker_id"`
	Token    string `json:"token"`
}

type Heartbeat struct {
	SentAt string `json:"sent_at"`
}
