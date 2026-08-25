package realtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestReplayRejectsStaleAndOversizedCatchUpCursors(t *testing.T) {
	for _, test := range []struct {
		name       string
		after, min int64
		max        int64
	}{
		{name: "below retained minimum boundary", after: 40, min: 42, max: 90},
		{name: "above catch-up window", after: 1, min: 1, max: MaxReplayEvents + 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryReplayStore{min: test.min, max: test.max}
			hub := NewHub()
			subscription := hub.Subscribe(1, test.after)
			err := NewReplayer(store, noopDrainer{}).CatchUp(context.Background(), subscription, test.after, func(Event) error {
				t.Fatal("stale cursor emitted an event")
				return nil
			})
			if !errors.Is(err, ErrResyncRequired) {
				t.Fatalf("CatchUp error = %v, want %v", err, ErrResyncRequired)
			}
			if store.pageCalls != 0 {
				t.Fatalf("replay page calls = %d, want 0", store.pageCalls)
			}
		})
	}
}

func TestReplayAcceptsRetainedMinimumBoundaryAndEqualCursor(t *testing.T) {
	for _, test := range []struct {
		name       string
		after, min int64
		max        int64
		want       []int64
	}{
		{name: "boundary M minus one", after: 41, min: 42, max: 43, want: []int64{42, 43}},
		{name: "equal to M", after: 42, min: 42, max: 43, want: []int64{43}},
		{name: "M equals one", after: 0, min: 1, max: 1, want: []int64{1}},
		{name: "empty log", after: 0, min: 0, max: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &memoryReplayStore{
				min: test.min, max: test.max, members: map[int64]map[int64]bool{1: {3: true}},
			}
			subscription := NewHub().Subscribe(1, test.after)
			var got []int64
			err := NewReplayer(store, noopDrainer{}).CatchUp(
				context.Background(), subscription, test.after, func(event Event) error {
					got = append(got, event.Seq)
					return nil
				},
			)
			if err != nil {
				t.Fatalf("CatchUp: %v", err)
			}
			assertSequences(t, got, test.want)
		})
	}
}

func TestInitialReplayUsesBoundedPagesAndMostRecentWindow(t *testing.T) {
	store := &memoryReplayStore{
		min: 1, max: 2 * MaxReplayEvents,
		members: map[int64]map[int64]bool{1: {3: true}},
	}
	hub := NewHub()
	subscription := hub.Subscribe(1, 0)
	var got []int64
	err := NewReplayer(store, noopDrainer{}).CatchUp(context.Background(), subscription, 0, func(event Event) error {
		got = append(got, event.Seq)
		return nil
	})
	if err != nil {
		t.Fatalf("CatchUp: %v", err)
	}
	if len(got) != MaxReplayEvents || got[0] != MaxReplayEvents+1 || got[len(got)-1] != 2*MaxReplayEvents {
		t.Fatalf("initial replay = len %d bounds %d..%d", len(got), got[0], got[len(got)-1])
	}
	if store.maxRequestedPage > ReplayPageSize {
		t.Fatalf("requested page = %d, want <= %d", store.maxRequestedPage, ReplayPageSize)
	}
}

func TestRegisteredThreeClientReplayHandoffIsAuthorizedOrderedAndExactlyOnce(t *testing.T) {
	store := &memoryReplayStore{
		min: 1, max: 1,
		members: map[int64]map[int64]bool{
			1: {3: true}, 2: {3: true}, 99: {},
		},
	}
	hub := NewHub()
	subscriptions := []*Subscription{
		hub.Subscribe(1, 0), hub.Subscribe(2, 0), hub.Subscribe(99, 0),
	}
	for _, subscription := range subscriptions {
		t.Cleanup(subscription.Close)
	}

	store.boundsHook = func() {
		store.mu.Lock()
		defer store.mu.Unlock()
		if store.max == 1 {
			hub.Publish(testEvent(2, `{}`))
			store.max = 2
		}
	}
	replayer := NewReplayer(store, noopDrainer{})
	received := make([][]int64, len(subscriptions))
	for index, subscription := range subscriptions {
		err := replayer.CatchUp(context.Background(), subscription, 0, func(event Event) error {
			received[index] = append(received[index], event.Seq)
			return nil
		})
		if err != nil {
			t.Fatalf("client %d CatchUp: %v", index, err)
		}
	}

	hub.Publish(testEvent(3, `{}`))
	for index := 0; index < 2; index++ {
		event, err := replayer.Next(context.Background(), subscriptions[index])
		if err != nil {
			t.Fatalf("client %d Next: %v", index, err)
		}
		received[index] = append(received[index], event.Event.Seq)
		assertSequences(t, received[index], []int64{1, 2, 3})
	}
	if len(received[2]) != 0 {
		t.Fatalf("outsider replay = %v, want none", received[2])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := replayer.Next(ctx, subscriptions[2]); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("outsider Next error = %v, want deadline with no event", err)
	}
}

func assertSequences(t *testing.T, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sequences = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("sequences = %v, want %v", got, want)
		}
	}
}

type memoryReplayStore struct {
	mu               sync.Mutex
	min, max         int64
	members          map[int64]map[int64]bool
	boundsHook       func()
	pageCalls        int
	maxRequestedPage int
}

func (s *memoryReplayStore) EventBounds(context.Context) (int64, int64, error) {
	if s.boundsHook != nil {
		s.boundsHook()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.min, s.max, nil
}

func (s *memoryReplayStore) ReplayEvents(
	_ context.Context, userID, afterSeq, throughSeq int64, limit int,
) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageCalls++
	if limit > s.maxRequestedPage {
		s.maxRequestedPage = limit
	}
	events := make([]Event, 0, limit)
	for seq := afterSeq + 1; seq <= throughSeq && len(events) < limit; seq++ {
		if s.members[userID][3] {
			events = append(events, testEvent(seq, `{}`))
		}
	}
	return events, nil
}

func (s *memoryReplayStore) Memberships(_ context.Context, userID int64) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	conversationIDs := make([]int64, 0, len(s.members[userID]))
	for conversationID, allowed := range s.members[userID] {
		if allowed {
			conversationIDs = append(conversationIDs, conversationID)
		}
	}
	return conversationIDs, nil
}

type noopDrainer struct{}

func (noopDrainer) DrainThrough(context.Context, int64) error { return nil }
