package realtime

import (
	"context"
	"errors"
	"fmt"
)

const (
	ReplayPageSize           = 500
	MaxReplayEvents          = 10_000
	MaxSubscriberMemberships = 10_000
)

var (
	ErrInvalidCursor  = errors.New("realtime cursor is invalid")
	ErrResyncRequired = errors.New("realtime cursor requires resynchronization")
)

// ReplayStore is the persistence boundary for membership-authorized replay and
// live-event filtering.
type ReplayStore interface {
	Memberships(context.Context, int64) ([]int64, error)
	EventBounds(context.Context) (minimum, highWater int64, err error)
	ReplayEvents(
		context.Context, int64, int64, int64, int,
	) ([]Event, error)
}

type EventDrainer interface {
	DrainThrough(context.Context, int64) error
}

type Replayer struct {
	store   ReplayStore
	drainer EventDrainer
}

func NewReplayer(store ReplayStore, drainer EventDrainer) *Replayer {
	return &Replayer{store: store, drainer: drainer}
}

// CatchUp captures the durable high-water mark after the caller has registered
// subscription, replays through it, and then exposes only newer buffered live
// events.
func (r *Replayer) CatchUp(
	ctx context.Context,
	subscription *Subscription,
	afterSeq int64,
	send func(Event) error,
) error {
	if r == nil || r.store == nil || r.drainer == nil || subscription == nil || subscription.UserID() <= 0 ||
		afterSeq < 0 || send == nil {
		return ErrInvalidCursor
	}
	memberships, err := r.store.Memberships(ctx, subscription.UserID())
	if err != nil {
		return err
	}
	subscription.SetMemberships(memberships)
	minimum, highWater, err := r.store.EventBounds(ctx)
	if err != nil {
		return err
	}
	if afterSeq > highWater {
		return ErrInvalidCursor
	}
	if afterSeq > 0 && minimum > 0 && afterSeq < minimum-1 {
		return ErrResyncRequired
	}
	if afterSeq > 0 && highWater-afterSeq > MaxReplayEvents {
		return ErrResyncRequired
	}
	if err := r.drainer.DrainThrough(ctx, highWater); err != nil {
		return err
	}

	cursor := afterSeq
	if afterSeq == 0 && highWater > MaxReplayEvents {
		cursor = highWater - MaxReplayEvents
	}
	emitted := 0
	for cursor < highWater {
		events, err := r.store.ReplayEvents(
			ctx, subscription.UserID(), cursor, highWater, ReplayPageSize,
		)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			break
		}
		for _, event := range events {
			if event.Seq <= cursor || event.Seq > highWater {
				return fmt.Errorf("replay store returned an unordered event")
			}
			cursor = event.Seq
			if event.ConversationID <= 0 {
				continue
			}
			emitted++
			if emitted > MaxReplayEvents {
				return ErrResyncRequired
			}
			if err := send(event); err != nil {
				return err
			}
		}
		if len(events) < ReplayPageSize {
			break
		}
	}
	subscription.FinishReplay(highWater)
	return nil
}

// Next waits for the next live event authorized by the subscription's bounded
// in-memory membership index.
func (r *Replayer) Next(ctx context.Context, subscription *Subscription) (Delivery, error) {
	if r == nil || subscription == nil {
		return Delivery{}, ErrInvalidCursor
	}
	return subscription.Next(ctx)
}
