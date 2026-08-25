package realtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPumpSafetyPollDrainsCommitWithoutNotify(t *testing.T) {
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
	source.append(testEvent(1, `{}`))

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	delivery, err := subscription.Next(ctx)
	if err != nil || delivery.Event.Seq != 1 {
		t.Fatalf("safety-poll delivery = (seq %d, %v), want seq 1", delivery.Event.Seq, err)
	}
}

func TestPumpCloseCancelsBlockedOrderedRead(t *testing.T) {
	source := newBlockedEventLog()
	pump, err := NewPump(source, NewHub())
	if err != nil {
		t.Fatalf("NewPump: %v", err)
	}
	pump.Notify(1)
	<-source.entered
	closed := make(chan struct{})
	go func() {
		pump.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(200 * time.Millisecond):
		close(source.release)
		<-closed
		t.Fatal("Pump.Close did not cancel blocked OrderedEvents")
	}
}

func TestPumpCloseInterruptsContinuouslyFullPages(t *testing.T) {
	source := &fullPageEventLog{started: make(chan struct{})}
	pump, err := NewPump(source, NewHub())
	if err != nil {
		t.Fatalf("NewPump: %v", err)
	}
	pump.Notify(1)
	<-source.started
	closed := make(chan struct{})
	go func() {
		pump.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(200 * time.Millisecond):
		source.halt.Store(true)
		<-closed
		t.Fatal("Pump.Close did not interrupt continuously full pages")
	}
}

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

type blockedEventLog struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockedEventLog() *blockedEventLog {
	return &blockedEventLog{entered: make(chan struct{}), release: make(chan struct{})}
}

func (*blockedEventLog) EventBounds(context.Context) (int64, int64, error) { return 0, 0, nil }

func (s *blockedEventLog) OrderedEvents(ctx context.Context, _ int64, _ int) ([]Event, error) {
	s.once.Do(func() { close(s.entered) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.release:
		return nil, nil
	}
}

type fullPageEventLog struct {
	started chan struct{}
	once    sync.Once
	halt    atomic.Bool
}

func (*fullPageEventLog) EventBounds(context.Context) (int64, int64, error) { return 0, 0, nil }

func (s *fullPageEventLog) OrderedEvents(
	ctx context.Context, afterSeq int64, limit int,
) ([]Event, error) {
	s.once.Do(func() { close(s.started) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.halt.Load() {
		return nil, nil
	}
	events := make([]Event, limit)
	for index := range events {
		events[index] = testEvent(afterSeq+int64(index)+1, `{}`)
	}
	return events, nil
}
