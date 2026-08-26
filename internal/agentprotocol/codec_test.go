package agentprotocol

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGoldenProtocolMessagesRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/messages.json")
	if err != nil {
		t.Fatal(err)
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatalf("decode fixture list: %v", err)
	}
	want := []Type{TypeNegotiate, TypeAuthenticate, TypeStart, TypeSteer, TypeInterrupt,
		TypeResume, TypeAnswer, TypeProgress, TypeApproval, TypeQuestion, TypeArtifact,
		TypeFailure, TypeUsage, TypeHeartbeat}
	if len(messages) != len(want) {
		t.Fatalf("fixtures = %d, want %d", len(messages), len(want))
	}
	for index, raw := range messages {
		envelope, err := Decode(raw)
		if err != nil {
			t.Fatalf("fixture %d: %v", index, err)
		}
		if envelope.Type != want[index] || envelope.Seq != uint64(index+1) {
			t.Fatalf("fixture %d envelope = %#v", index, envelope)
		}
		encoded, err := Encode(envelope)
		if err != nil {
			t.Fatalf("encode fixture %d: %v", index, err)
		}
		if _, err := Decode(encoded); err != nil {
			t.Fatalf("round-trip fixture %d: %v", index, err)
		}
	}
}

func TestDecodeRejectsUnknownOrUnsafeMessages(t *testing.T) {
	valid := `{"version":1,"seq":1,"type":"heartbeat","payload":{"sent_at":"2026-08-26T10:00:00Z"}}`
	cases := []string{
		`{"version":1,"seq":1,"type":"future","payload":{}}`,
		`{"version":1,"seq":1,"type":"heartbeat","payload":{"sent_at":"2026-08-26T10:00:00Z","extra":true}}`,
		`{"version":1,"seq":1,"type":"heartbeat","payload":{"sent_at":"bad"}}`,
		`{"version":2,"seq":1,"type":"heartbeat","payload":{"sent_at":"2026-08-26T10:00:00Z"}}`,
		`{"version":1,"seq":0,"type":"heartbeat","payload":{"sent_at":"2026-08-26T10:00:00Z"}}`,
		valid + ` {}`,
		strings.Repeat(" ", MaxEnvelopeBytes+1),
	}
	for _, input := range cases {
		if _, err := Decode([]byte(input)); err == nil {
			t.Fatalf("Decode(%q) error = nil", input[:min(len(input), 80)])
		}
	}
}

func TestEnvelopeStringRedactsPayload(t *testing.T) {
	envelope, err := Decode([]byte(`{"version":1,"seq":1,"type":"authenticate","payload":{"worker_id":"worker-1","token":"secret-value"}}`))
	if err != nil {
		t.Fatal(err)
	}
	text := envelope.String()
	if strings.Contains(text, "secret-value") || !strings.Contains(text, "authenticate") {
		t.Fatalf("String() = %q", text)
	}
}

func FuzzDecode(f *testing.F) {
	data, _ := os.ReadFile("testdata/messages.json")
	f.Add(data)
	f.Add([]byte(`{"version":1}`))
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > MaxEnvelopeBytes+1 {
			input = input[:MaxEnvelopeBytes+1]
		}
		envelope, err := Decode(input)
		if err != nil {
			return
		}
		encoded, err := Encode(envelope)
		if err != nil {
			t.Fatalf("accepted envelope cannot encode: %v", err)
		}
		if !json.Valid(encoded) || bytes.Contains(encoded, []byte("secret-value")) && envelope.Type != TypeAuthenticate {
			t.Fatalf("invalid encoding")
		}
	})
}
