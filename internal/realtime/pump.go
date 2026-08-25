package realtime

import (
	"context"
	"errors"
	"sync"
)

const PumpPageSize = 500

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
	stop   chan struct{}
	done   chan struct{}

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
	_, highWater, err := source.EventBounds(context.Background())
	if err != nil {
		return nil, err
	}
	pump := &Pump{
		source: source, hub: hub, cursor: highWater,
		wake: make(chan struct{}, 1), stop: make(chan struct{}),
		done: make(chan struct{}), updated: make(chan struct{}),
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
	close(p.stop)
	p.broadcastLocked()
	p.mu.Unlock()
	<-p.done
}

func (p *Pump) run() {
	defer close(p.done)
	for {
		select {
		case <-p.stop:
			return
		case <-p.wake:
			p.drain()
		}
	}
}

func (p *Pump) drain() {
	for {
		p.mu.Lock()
		cursor := p.cursor
		p.mu.Unlock()
		events, err := p.source.OrderedEvents(context.Background(), cursor, PumpPageSize)
		if err != nil {
			p.setError(err)
			return
		}
		if len(events) == 0 {
			return
		}
		for _, event := range events {
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

func (p *Pump) broadcastLocked() {
	close(p.updated)
	p.updated = make(chan struct{})
}
