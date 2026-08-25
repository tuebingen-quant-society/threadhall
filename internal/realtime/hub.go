package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
)

const (
	MaxQueuedEvents = 256
	MaxQueuedBytes  = 1 << 20
)

var (
	ErrSlowClient         = errors.New("realtime client queue overflowed")
	ErrHubClosed          = errors.New("realtime hub is closed")
	ErrSubscriptionClosed = errors.New("realtime subscription is closed")
)

// Delivery contains one event and its stable, pre-serialized wire envelope.
type Delivery struct {
	Event Event
	Data  []byte
}

// Hub fans committed events out through bounded, nonblocking subscriptions.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[*Subscription]struct{}
	closed      bool
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[*Subscription]struct{})}
}

// Subscribe completes registration synchronously before returning.
func (h *Hub) Subscribe(userID, afterSeq int64) *Subscription {
	subscription := &Subscription{
		hub: h, userID: userID, lastSeen: afterSeq,
		ready: make(chan struct{}, 1), done: make(chan struct{}),
	}
	h.mu.Lock()
	if h.closed {
		subscription.closeLocked(ErrHubClosed)
	} else {
		h.subscribers[subscription] = struct{}{}
	}
	h.mu.Unlock()
	return subscription
}

// Publish never waits for a subscriber. Invalid or unscoped core events are
// not placed on any socket queue.
func (h *Hub) Publish(event Event) {
	if event.Seq <= 0 || event.ConversationID <= 0 {
		return
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return
	}
	delivery := Delivery{Event: event, Data: encoded}

	h.mu.RLock()
	subscriptions := make([]*Subscription, 0, len(h.subscribers))
	for subscription := range h.subscribers {
		subscriptions = append(subscriptions, subscription)
	}
	h.mu.RUnlock()
	for _, subscription := range subscriptions {
		subscription.enqueue(delivery)
	}
}

func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	subscriptions := make([]*Subscription, 0, len(h.subscribers))
	for subscription := range h.subscribers {
		subscriptions = append(subscriptions, subscription)
	}
	h.subscribers = make(map[*Subscription]struct{})
	h.mu.Unlock()
	for _, subscription := range subscriptions {
		subscription.fail(ErrHubClosed, false)
	}
}

func (h *Hub) remove(subscription *Subscription) {
	h.mu.Lock()
	delete(h.subscribers, subscription)
	h.mu.Unlock()
}

// Subscription is one bounded outbound event queue.
type Subscription struct {
	hub    *Hub
	userID int64

	mu          sync.Mutex
	queue       []Delivery
	queuedBytes int
	lastSeen    int64
	memberships map[int64]struct{}
	live        bool
	err         error
	ready       chan struct{}
	done        chan struct{}
}

func (s *Subscription) UserID() int64         { return s.userID }
func (s *Subscription) Done() <-chan struct{} { return s.done }

// SetMemberships installs the subscriber's one-time current membership
// snapshot. Ordered membership events keep it current after replay.
func (s *Subscription) SetMemberships(conversationIDs []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memberships = make(map[int64]struct{}, len(conversationIDs))
	for _, conversationID := range conversationIDs {
		if conversationID > 0 {
			s.memberships[conversationID] = struct{}{}
		}
	}
}

func (s *Subscription) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Subscription) Next(ctx context.Context) (Delivery, error) {
	for {
		select {
		case <-ctx.Done():
			return Delivery{}, ctx.Err()
		case <-s.done:
			return Delivery{}, s.Err()
		case <-s.ready:
			s.mu.Lock()
			if len(s.queue) == 0 {
				err := s.err
				s.mu.Unlock()
				if err != nil {
					return Delivery{}, err
				}
				continue
			}
			delivery := s.queue[0]
			s.queue[0] = Delivery{}
			s.queue = s.queue[1:]
			s.queuedBytes -= len(delivery.Data)
			if len(s.queue) > 0 {
				s.signalReady()
			}
			s.mu.Unlock()
			return delivery, nil
		}
	}
}

// FinishReplay drops live publications already covered by the database mark.
func (s *Subscription) FinishReplay(mark int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.queue[:0]
	queuedBytes := 0
	for _, delivery := range s.queue {
		if delivery.Event.Seq <= mark {
			s.applyMembershipChanges(delivery.Event)
			continue
		}
		if s.authorizeLive(delivery.Event) {
			kept = append(kept, delivery)
			queuedBytes += len(delivery.Data)
		}
	}
	s.queue = kept
	s.queuedBytes = queuedBytes
	if mark > s.lastSeen {
		s.lastSeen = mark
	}
	s.live = true
	if len(s.queue) > 0 {
		s.signalReady()
	}
}

func (s *Subscription) Close() { s.fail(ErrSubscriptionClosed, true) }

func (s *Subscription) enqueue(delivery Delivery) {
	s.mu.Lock()
	if s.err != nil || delivery.Event.Seq <= s.lastSeen {
		s.mu.Unlock()
		return
	}
	s.lastSeen = delivery.Event.Seq
	if s.live && !s.authorizeLive(delivery.Event) {
		s.mu.Unlock()
		return
	}
	if len(s.queue)+1 > MaxQueuedEvents || s.queuedBytes+len(delivery.Data) > MaxQueuedBytes {
		s.closeLocked(ErrSlowClient)
		s.mu.Unlock()
		s.hub.remove(s)
		return
	}
	s.queue = append(s.queue, delivery)
	s.queuedBytes += len(delivery.Data)
	s.signalReady()
	s.mu.Unlock()
}

func (s *Subscription) authorizeLive(event Event) bool {
	_, allowedBefore := s.memberships[event.ConversationID]
	for _, change := range event.MembershipChanges {
		if change.UserID == s.userID && change.Joined {
			s.memberships[event.ConversationID] = struct{}{}
		}
	}
	_, allowedAfterAdds := s.memberships[event.ConversationID]
	for _, change := range event.MembershipChanges {
		if change.UserID == s.userID && !change.Joined {
			delete(s.memberships, event.ConversationID)
		}
	}
	return allowedBefore || allowedAfterAdds
}

func (s *Subscription) applyMembershipChanges(event Event) {
	for _, change := range event.MembershipChanges {
		if change.UserID != s.userID {
			continue
		}
		if change.Joined {
			s.memberships[event.ConversationID] = struct{}{}
		} else {
			delete(s.memberships, event.ConversationID)
		}
	}
}

func (s *Subscription) fail(err error, remove bool) {
	s.mu.Lock()
	s.closeLocked(err)
	s.mu.Unlock()
	if remove {
		s.hub.remove(s)
	}
}

func (s *Subscription) closeLocked(err error) {
	if s.err != nil {
		return
	}
	s.err = err
	s.queue = nil
	s.queuedBytes = 0
	close(s.done)
}

func (s *Subscription) signalReady() {
	select {
	case s.ready <- struct{}{}:
	default:
	}
}
