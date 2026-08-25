package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestHubDisconnectsSubscriberAtEventCountBudgetWithoutBlockingPublish(t *testing.T) {
	hub := NewHub()
	subscription := hub.Subscribe(7, 0)
	t.Cleanup(subscription.Close)

	for seq := int64(1); seq <= MaxQueuedEvents; seq++ {
		hub.Publish(testEvent(seq, `{}`))
	}
	select {
	case <-subscription.Done():
		t.Fatal("subscriber closed at, rather than above, the event-count budget")
	default:
	}

	returned := make(chan struct{})
	go func() {
		hub.Publish(testEvent(MaxQueuedEvents+1, `{}`))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on an overflowing subscriber")
	}
	if !errors.Is(subscription.Err(), ErrSlowClient) {
		t.Fatalf("subscriber error = %v, want %v", subscription.Err(), ErrSlowClient)
	}
}

func TestHubDisconnectsSubscriberAboveSerializedByteBudget(t *testing.T) {
	hub := NewHub()
	subscription := hub.Subscribe(8, 0)
	t.Cleanup(subscription.Close)
	payload := fmt.Sprintf(`{"body":%q}`, strings.Repeat("x", 5000))

	for seq := int64(1); seq <= MaxQueuedEvents; seq++ {
		hub.Publish(testEvent(seq, payload))
		if subscription.Err() != nil {
			break
		}
	}
	if !errors.Is(subscription.Err(), ErrSlowClient) {
		t.Fatalf("subscriber error = %v, want byte-budget overflow", subscription.Err())
	}
}

func TestHubSuppressesDuplicateAndDescendingLiveSequences(t *testing.T) {
	hub := NewHub()
	subscription := hub.Subscribe(9, 1)
	t.Cleanup(subscription.Close)

	for _, seq := range []int64{1, 2, 2, 1, 3} {
		hub.Publish(testEvent(seq, `{}`))
	}
	for _, want := range []int64{2, 3} {
		delivery, err := subscription.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if delivery.Event.Seq != want {
			t.Fatalf("sequence = %d, want %d", delivery.Event.Seq, want)
		}
	}
	select {
	case <-subscription.ready:
		t.Fatal("duplicate or descending sequence remained queued")
	default:
	}
}

func TestHubCloseReleasesSubscribers(t *testing.T) {
	hub := NewHub()
	subscription := hub.Subscribe(10, 0)
	hub.Close()
	if _, err := subscription.Next(context.Background()); !errors.Is(err, ErrHubClosed) {
		t.Fatalf("Next error = %v, want %v", err, ErrHubClosed)
	}
	if got := hub.Subscribe(11, 0).Err(); !errors.Is(got, ErrHubClosed) {
		t.Fatalf("late subscription error = %v, want %v", got, ErrHubClosed)
	}
}

func testEvent(seq int64, payload string) Event {
	return Event{
		Seq: seq, Type: "message.sent", ConversationID: 3,
		EntityID: seq, Payload: json.RawMessage(payload),
	}
}
