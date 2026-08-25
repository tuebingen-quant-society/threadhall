package realtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

const PumpPageSize = 500
const PumpPollInterval = 100 * time.Millisecond

var ErrPumpClosed = errors.New("realtime event pump is closed")

// EventLog is the durable global sequence consumed by Pump.
type EventLog interface {
	EventBounds(context.Context) (minimum, highWater int64, err error)
	OrderedEvents(context.Context, int64, int) ([]Event, error)
}

// Pump is the single ordered bridge from committed SQLite events to Hub.
type Pump struct {
	source EventLog
	hub    *Hub
	wake   chan struct{}
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	cursor  int64
	err     error
	updated chan struct{}
	closed  bool
}

func NewPump(source EventLog, hub *Hub) (*Pump, error) {
	if source == nil || hub == nil {
		return nil, errors.New("event log and hub are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, highWater, err := source.EventBounds(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	pump := &Pump{
		source: source, hub: hub, cursor: highWater,
		wake: make(chan struct{}, 1), done: make(chan struct{}),
		ctx: ctx, cancel: cancel, updated: make(chan struct{}),
	}
	go pump.run()
	return pump, nil
}

// Notify only wakes the tailer; the supplied sequence never determines order.
func (p *Pump) Notify(_ int64) {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *Pump) DrainThrough(ctx context.Context, sequence int64) error {
	if sequence < 0 {
		return ErrInvalidCursor
	}
	p.Notify(sequence)
	for {
		p.mu.Lock()
		if p.cursor >= sequence {
			p.mu.Unlock()
			return nil
		}
		if p.closed {
			p.mu.Unlock()
			return ErrPumpClosed
		}
		if p.err != nil {
			err := p.err
			p.mu.Unlock()
			return err
		}
		updated := p.updated
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-updated:
		}
	}
}

func (p *Pump) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		<-p.done
		return
	}
	p.closed = true
	p.cancel()
	p.broadcastLocked()
	p.mu.Unlock()
	<-p.done
}

func (p *Pump) run() {
	defer close(p.done)
	ticker := time.NewTicker(PumpPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.wake:
			p.drain()
		case <-ticker.C:
			p.drain()
		}
	}
}

func (p *Pump) drain() {
	for {
		if p.ctx.Err() != nil {
			return
		}
		p.mu.Lock()
		cursor := p.cursor
		p.mu.Unlock()
		events, err := p.source.OrderedEvents(p.ctx, cursor, PumpPageSize)
		if err != nil {
			if p.ctx.Err() != nil {
				return
			}
			p.setError(err)
			return
		}
		p.clearError()
		if len(events) == 0 {
			return
		}
		for _, event := range events {
			if p.ctx.Err() != nil {
				return
			}
			if event.Seq <= cursor {
				p.setError(errors.New("event log returned an unordered event"))
				return
			}
			p.hub.Publish(event)
			cursor = event.Seq
			p.mu.Lock()
			p.cursor = cursor
			p.err = nil
			p.broadcastLocked()
			p.mu.Unlock()
		}
		if len(events) < PumpPageSize {
			return
		}
	}
}

func (p *Pump) setError(err error) {
	p.mu.Lock()
	p.err = err
	p.broadcastLocked()
	p.mu.Unlock()
}

func (p *Pump) clearError() {
	p.mu.Lock()
	if p.err != nil {
		p.err = nil
		p.broadcastLocked()
	}
	p.mu.Unlock()
}

func (p *Pump) broadcastLocked() {
	close(p.updated)
	p.updated = make(chan struct{})
}
