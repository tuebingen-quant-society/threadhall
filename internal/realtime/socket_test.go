package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSocketReplaysStableEventEnvelope(t *testing.T) {
	store := &memoryReplayStore{
		min: 1, max: 1, members: map[int64]map[int64]bool{1: {3: true}},
	}
	connection := dialTestSocket(t, store, NewHub(), 0)
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil || event.Seq != 1 ||
		event.Type != "message.sent" || event.ConversationID != 3 {
		t.Fatalf("event envelope = (%s, %v)", data, err)
	}
}

func TestSocketEmitsResyncRequiredForStaleCursor(t *testing.T) {
	store := &memoryReplayStore{
		min: 3, max: 3, members: map[int64]map[int64]bool{1: {3: true}},
	}
	connection := dialTestSocket(t, store, NewHub(), 1)
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := connection.Read(ctx)
	if err != nil || string(data) != `{"type":"resync_required"}` {
		t.Fatalf("resync envelope = (%s, %v)", data, err)
	}
	_, _, err = connection.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("close status = %d (%v), want %d", websocket.CloseStatus(err), err, websocket.StatusPolicyViolation)
	}
}

func TestSocketClosesOversizedInboundFrameWithStableReason(t *testing.T) {
	store := &memoryReplayStore{members: map[int64]map[int64]bool{1: {3: true}}}
	connection := dialTestSocket(t, store, NewHub(), 0)
	defer connection.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := connection.Write(ctx, websocket.MessageText, []byte(strings.Repeat("x", MaxInboundFrameBytes+1))); err != nil {
		t.Fatalf("Write oversized frame: %v", err)
	}
	_, _, err := connection.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig || !strings.Contains(err.Error(), CloseReasonFrameTooLarge) {
		t.Fatalf("oversized close = %v", err)
	}
}

func TestSocketOverflowDuringHighWaterPauseRequiresResync(t *testing.T) {
	registered := make(chan struct{})
	release := make(chan struct{})
	store := &memoryReplayStore{
		members: map[int64]map[int64]bool{1: {3: true}},
		boundsHook: func() {
			close(registered)
			<-release
		},
	}
	hub := NewHub()
	connection := dialTestSocket(t, store, hub, 0)
	defer connection.CloseNow()
	<-registered
	for seq := int64(1); seq <= MaxQueuedEvents+1; seq++ {
		hub.Publish(testEvent(seq, `{}`))
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, data, err := connection.Read(ctx)
	if err != nil || string(data) != `{"type":"resync_required"}` {
		t.Fatalf("overflow envelope = (%s, %v)", data, err)
	}
	_, _, err = connection.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("overflow close = %v", err)
	}
}

func dialTestSocket(t *testing.T, store ReplayStore, hub *Hub, afterSeq int64) *websocket.Conn {
	t.Helper()
	socket := NewSocket(hub, NewReplayer(store, noopDrainer{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		socket.Serve(w, request, 1, afterSeq)
	}))
	t.Cleanup(server.Close)
	connection, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	return connection
}
