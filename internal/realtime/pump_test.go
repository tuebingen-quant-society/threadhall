package realtime

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPumpDrainsDurableEventsInSequenceDespiteReversedNotifications(t *testing.T) {
	source := &memoryEventLog{}
	hub := NewHub()
	pump, err := NewPump(source, hub)
	if err != nil {
		t.Fatalf("NewPump: %v", err)
	}
	t.Cleanup(pump.Close)
	subscription := hub.Subscribe(1, 0)
	subscription.SetMemberships([]int64{3})
	subscription.FinishReplay(0)

	source.append(testEvent(1, `{}`), testEvent(2, `{}`))
	pump.Notify(2)
	pump.Notify(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pump.DrainThrough(ctx, 2); err != nil {
		t.Fatalf("DrainThrough: %v", err)
	}
	for _, want := range []int64{1, 2} {
		delivery, err := subscription.Next(ctx)
		if err != nil || delivery.Event.Seq != want {
			t.Fatalf("Next = (seq %d, %v), want %d", delivery.Event.Seq, err, want)
		}
	}
}

func TestPumpAppliesMembershipRemovalBeforeLaterMessage(t *testing.T) {
	source := &memoryEventLog{}
	hub := NewHub()
	pump, err := NewPump(source, hub)
	if err != nil {
		t.Fatalf("NewPump: %v", err)
	}
	t.Cleanup(pump.Close)
	subscription := hub.Subscribe(1, 0)
	subscription.SetMemberships([]int64{3})
	subscription.FinishReplay(0)

	removal := Event{
		Seq: 1, Type: "conversation.member_removed", ConversationID: 3,
		EntityID: 3, Payload: []byte(`[3,1]`),
		MembershipChanges: []MembershipChange{{UserID: 1, Joined: false}},
	}
	source.append(removal, testEvent(2, `{}`))
	pump.Notify(2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pump.DrainThrough(ctx, 2); err != nil {
		t.Fatalf("DrainThrough: %v", err)
	}
	delivery, err := subscription.Next(ctx)
	if err != nil || delivery.Event.Seq != 1 {
		t.Fatalf("removal delivery = (seq %d, %v), want seq 1", delivery.Event.Seq, err)
	}

	quiet, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if delivery, err := subscription.Next(quiet); err == nil {
		t.Fatalf("revoked subscriber received later sequence %d", delivery.Event.Seq)
	}
}

type memoryEventLog struct {
	mu     sync.Mutex
	events []Event
}

func (s *memoryEventLog) append(events ...Event) {
	s.mu.Lock()
	s.events = append(s.events, events...)
	s.mu.Unlock()
}

func (s *memoryEventLog) EventBounds(context.Context) (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.events) == 0 {
		return 0, 0, nil
	}
	return s.events[0].Seq, s.events[len(s.events)-1].Seq, nil
}

func (s *memoryEventLog) OrderedEvents(
	_ context.Context, afterSeq int64, limit int,
) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]Event, 0, limit)
	for _, event := range s.events {
		if event.Seq > afterSeq && len(events) < limit {
			events = append(events, event)
		}
	}
	return events, nil
}
