// Package agentprotocol defines the bounded provider-neutral wire contract
// between Threadhall and outbound agent workers.
package agentprotocol

const (
	Version          uint16 = 1
	MaxEnvelopeBytes        = 256 << 10
	maxIDBytes              = 128
	maxSummaryBytes         = 2048
)

type Type string

const (
	TypeNegotiate    Type = "negotiate"
	TypeAuthenticate Type = "authenticate"
	TypeStart        Type = "start"
	TypeSteer        Type = "steer"
	TypeInterrupt    Type = "interrupt"
	TypeResume       Type = "resume"
	TypeAnswer       Type = "answer"
	TypeProgress     Type = "progress"
	TypeApproval     Type = "approval"
	TypeQuestion     Type = "question"
	TypeArtifact     Type = "artifact"
	TypeFailure      Type = "failure"
	TypeUsage        Type = "usage"
	TypeHeartbeat    Type = "heartbeat"
)

func (t Type) known() bool {
	switch t {
	case TypeNegotiate, TypeAuthenticate, TypeStart, TypeSteer, TypeInterrupt,
		TypeResume, TypeAnswer, TypeProgress, TypeApproval, TypeQuestion,
		TypeArtifact, TypeFailure, TypeUsage, TypeHeartbeat:
		return true
	default:
		return false
	}
}
