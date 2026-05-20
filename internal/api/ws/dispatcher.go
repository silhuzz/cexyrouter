package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type Notification struct {
	ID         int64     `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	EventType  string    `json:"event_type"`
	ExchangeID int32     `json:"exchange_id"`
	CoinID     int32     `json:"coin_id"`
	ChainID    int32     `json:"chain_id"`
}

type Listener interface {
	Listen(ctx context.Context, notifications chan<- Notification) error
}

type Dispatcher struct {
	lookup         EventLookup
	outboundBuffer int

	mu      sync.Mutex
	nextID  int64
	clients map[int64]client
}

type client struct {
	filters Filters
	out     chan envelope
}

func NewDispatcher(lookup EventLookup) *Dispatcher {
	return &Dispatcher{
		lookup:         lookup,
		outboundBuffer: defaultOutboundBuffer,
		clients:        make(map[int64]client),
	}
}

func (d *Dispatcher) Subscribe(filters Filters) (<-chan envelope, func(), error) {
	if d == nil {
		return nil, nil, fmt.Errorf("dispatcher is nil")
	}
	normalized, err := NormalizeFilters(filters)
	if err != nil {
		return nil, nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.clients == nil {
		d.clients = make(map[int64]client)
	}
	d.nextID++
	id := d.nextID
	out := make(chan envelope, d.outboundBuffer)
	d.clients[id] = client{filters: normalized, out: out}

	cancel := func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		if client, ok := d.clients[id]; ok {
			delete(d.clients, id)
			close(client.out)
		}
	}
	return out, cancel, nil
}

func (d *Dispatcher) DispatchNotification(ctx context.Context, notification Notification) error {
	if d == nil {
		return fmt.Errorf("dispatcher is nil")
	}
	if d.lookup == nil {
		return fmt.Errorf("event lookup is required")
	}
	event, err := d.lookup.EventByID(ctx, notification.ID)
	if err != nil {
		return err
	}
	d.Publish(event)
	return nil
}

func (d *Dispatcher) Publish(event Event) {
	if d == nil {
		return
	}
	if event.Cursor == "" {
		if withCursor, err := attachCursor(event); err == nil {
			event = withCursor
		}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	for id, client := range d.clients {
		if !client.filters.Match(event) {
			continue
		}
		select {
		case client.out <- envelope{Type: "event", Event: event}:
		default:
			delete(d.clients, id)
			close(client.out)
		}
	}
}

func ParseNotification(payload []byte) (Notification, error) {
	var notification Notification
	if err := json.Unmarshal(payload, &notification); err != nil {
		return Notification{}, fmt.Errorf("parse notification: %w", err)
	}
	if notification.ID <= 0 {
		return Notification{}, fmt.Errorf("notification id must be positive")
	}
	return notification, nil
}
