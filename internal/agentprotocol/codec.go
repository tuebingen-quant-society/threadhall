package agentprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidEnvelope = errors.New("invalid agent protocol envelope")

func Decode(data []byte) (Envelope, error) {
	if len(data) == 0 || len(data) > MaxEnvelopeBytes {
		return Envelope{}, ErrInvalidEnvelope
	}
	var envelope Envelope
	if err := decodeStrict(data, &envelope); err != nil || envelope.Version != Version ||
		envelope.Seq == 0 || !envelope.Type.known() || len(envelope.Payload) == 0 {
		return Envelope{}, ErrInvalidEnvelope
	}
	if err := validatePayload(envelope.Type, envelope.Payload); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrInvalidEnvelope, err)
	}
	return envelope, nil
}

func Encode(envelope Envelope) ([]byte, error) {
	if envelope.Version != Version || envelope.Seq == 0 || !envelope.Type.known() ||
		validatePayload(envelope.Type, envelope.Payload) != nil {
		return nil, ErrInvalidEnvelope
	}
	data, err := json.Marshal(envelope)
	if err != nil || len(data) > MaxEnvelopeBytes {
		return nil, ErrInvalidEnvelope
	}
	return data, nil
}

func DecodePayload[T any](envelope Envelope) (T, error) {
	var value T
	if err := decodeStrict(envelope.Payload, &value); err != nil {
		return value, ErrInvalidEnvelope
	}
	return value, nil
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func validatePayload(kind Type, raw json.RawMessage) error {
	var value any
	switch kind {
	case TypeNegotiate:
		value = &Negotiate{}
	case TypeAuthenticate:
		value = &Authenticate{}
	case TypeStart:
		value = &Start{}
	case TypeSteer:
		value = &Steer{}
	case TypeInterrupt:
		value = &Interrupt{}
	case TypeResume:
		value = &Resume{}
	case TypeAnswer:
		value = &Answer{}
	case TypeProgress:
		value = &Progress{}
	case TypeApproval:
		value = &Approval{}
	case TypeQuestion:
		value = &Question{}
	case TypeArtifact:
		value = &Artifact{}
	case TypeFailure:
		value = &Failure{}
	case TypeUsage:
		value = &Usage{}
	case TypeHeartbeat:
		value = &Heartbeat{}
	default:
		return errors.New("unknown type")
	}
	if err := decodeStrict(raw, value); err != nil {
		return err
	}
	return validateValue(kind, value)
}

func validText(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= maximum
}

func validID(value string) bool   { return validText(value, maxIDBytes) }
func validTime(value string) bool { _, err := time.Parse(time.RFC3339, value); return err == nil }

func validateValue(kind Type, raw any) error {
	switch value := raw.(type) {
	case *Negotiate:
		if value.Minimum == 0 || value.Minimum > value.Maximum || value.Minimum > Version || value.Maximum < Version || len(value.Capabilities) > 32 {
			return ErrInvalidEnvelope
		}
		for _, item := range value.Capabilities {
			if !validText(item, 64) {
				return ErrInvalidEnvelope
			}
		}
	case *Authenticate:
		if !validID(value.WorkerID) || !validText(value.Token, 256) {
			return ErrInvalidEnvelope
		}
	case *Start:
		if !validID(value.TaskID) || value.AgentID <= 0 || value.ConversationID <= 0 || value.ThreadRootID <= 0 || !validText(value.Prompt, 32<<10) || len(value.Context) > 100 {
			return ErrInvalidEnvelope
		}
		for _, item := range value.Context {
			if !validContext(item) {
				return ErrInvalidEnvelope
			}
		}
	case *Steer:
		if !validID(value.TaskID) || !validText(value.Message, 16<<10) {
			return ErrInvalidEnvelope
		}
	case *Interrupt:
		if !validID(value.TaskID) {
			return ErrInvalidEnvelope
		}
	case *Resume:
		if !validID(value.TaskID) || !validID(value.SessionID) {
			return ErrInvalidEnvelope
		}
	case *Answer:
		if !validID(value.TaskID) || !validID(value.InteractionID) || len(value.Answers) == 0 || len(value.Answers) > 3 {
			return ErrInvalidEnvelope
		}
		for _, answer := range value.Answers {
			if !validID(answer.QuestionID) || len(answer.Value) == 0 || len(answer.Value) > 10 {
				return ErrInvalidEnvelope
			}
			for _, item := range answer.Value {
				if !validText(item, 512) {
					return ErrInvalidEnvelope
				}
			}
		}
	case *Progress:
		if !validID(value.TaskID) || !validText(value.Phase, 64) || !validText(value.Summary, maxSummaryBytes) || value.Percent < 0 || value.Percent > 100 {
			return ErrInvalidEnvelope
		}
	case *Approval:
		if !validID(value.TaskID) || !validID(value.InteractionID) || !validApprovalAction(value.Action) || !validText(value.Summary, maxSummaryBytes) || !validText(value.Digest, 256) || !validTime(value.ExpiresAt) {
			return ErrInvalidEnvelope
		}
	case *Question:
		if !validQuestion(value) {
			return ErrInvalidEnvelope
		}
	case *Artifact:
		if !validID(value.TaskID) || !validID(value.ArtifactID) || !validText(value.Name, 255) || strings.ContainsAny(value.Name, "/\\") || !validText(value.MediaType, 128) || value.Size < 0 || value.Size > 1<<30 {
			return ErrInvalidEnvelope
		}
	case *Failure:
		if !validID(value.TaskID) || !validText(value.Code, 64) || !validText(value.Detail, maxSummaryBytes) {
			return ErrInvalidEnvelope
		}
	case *Usage:
		if !validID(value.TaskID) || value.InputTokens < 0 || value.OutputTokens < 0 {
			return ErrInvalidEnvelope
		}
	case *Heartbeat:
		if !validTime(value.SentAt) {
			return ErrInvalidEnvelope
		}
	default:
		return ErrInvalidEnvelope
	}
	return nil
}

func validContext(item ContextItem) bool {
	return (item.Kind == "message" || item.Kind == "attachment" || item.Kind == "reference") && validID(item.ID) && validText(item.Content, 32<<10)
}

func validApprovalAction(value string) bool {
	switch value {
	case "network", "push", "pull_request", "merge", "destructive", "external_message":
		return true
	default:
		return false
	}
}

func validQuestion(value *Question) bool {
	if !validID(value.TaskID) || !validID(value.InteractionID) || len(value.Questions) == 0 || len(value.Questions) > 3 || !validTime(value.ExpiresAt) {
		return false
	}
	for _, question := range value.Questions {
		if !validID(question.ID) || !validText(question.Prompt, 1024) || (question.Kind != "single" && question.Kind != "multi" && question.Kind != "confirm" && question.Kind != "text") || len(question.Options) > 20 {
			return false
		}
		for _, option := range question.Options {
			if !validID(option.ID) || !validText(option.Label, 256) {
				return false
			}
		}
	}
	return true
}
