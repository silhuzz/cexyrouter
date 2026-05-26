package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestDispatchNotificationUsesCompletePayload(t *testing.T) {
	lookup := &countingLookup{}
	dispatcher := NewDispatcher(lookup)
	out, unsubscribe, err := dispatcher.Subscribe(Filters{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer unsubscribe()

	notification := completeNotification()
	if err := dispatcher.DispatchNotification(context.Background(), notification); err != nil {
		t.Fatalf("DispatchNotification: %v", err)
	}
	if lookup.calls != 0 {
		t.Fatalf("EventByID calls = %d, want 0", lookup.calls)
	}

	select {
	case msg := <-out:
		if msg.Event.ID != notification.ID || msg.Event.RailID != notification.RailID {
			t.Fatalf("unexpected event: %+v", msg.Event)
		}
		if msg.Event.Cursor == "" {
			t.Fatalf("event cursor was not attached")
		}
		if msg.Event.Summary == "" {
			t.Fatalf("event summary was not built")
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for dispatched event")
	}
}

func TestDispatchNotificationFallsBackForPartialPayload(t *testing.T) {
	lookup := &countingLookup{event: Event{
		ID:         42,
		RailID:     7,
		EventType:  "deposit_off",
		Exchange:   ExchangeRef{Ref: Ref{ID: 1, Slug: "binance"}, Name: "Binance", Region: "Global"},
		Coin:       CoinRef{Ref: Ref{ID: 2, Slug: "usdt"}, Symbol: "USDT", Name: "Tether USD"},
		Chain:      ChainRef{Ref: Ref{ID: 3, Slug: "tron"}, Symbol: "TRX", Name: "TRON"},
		Before:     json.RawMessage(`{"deposit_enabled":true}`),
		After:      json.RawMessage(`{"deposit_enabled":false}`),
		OccurredAt: time.Date(2026, 5, 26, 1, 2, 3, 0, time.UTC),
	}}
	dispatcher := NewDispatcher(lookup)

	if err := dispatcher.DispatchNotification(context.Background(), Notification{ID: 42}); err != nil {
		t.Fatalf("DispatchNotification: %v", err)
	}
	if lookup.calls != 1 {
		t.Fatalf("EventByID calls = %d, want 1", lookup.calls)
	}
}

func completeNotification() Notification {
	return Notification{
		ID:         42,
		RailID:     7,
		EventType:  "deposit_off",
		OccurredAt: time.Date(2026, 5, 26, 1, 2, 3, 0, time.UTC),
		ExchangeID: 1,
		CoinID:     2,
		ChainID:    3,
		Exchange:   ExchangeRef{Ref: Ref{ID: 1, Slug: "binance"}, Name: "Binance", Region: "Global"},
		Coin:       CoinRef{Ref: Ref{ID: 2, Slug: "usdt"}, Symbol: "USDT", Name: "Tether USD"},
		Chain:      ChainRef{Ref: Ref{ID: 3, Slug: "tron"}, Symbol: "TRX", Name: "TRON"},
		Before:     json.RawMessage(`{"deposit_enabled":true}`),
		After:      json.RawMessage(`{"deposit_enabled":false}`),
	}
}

type countingLookup struct {
	calls int
	event Event
	err   error
}

func (l *countingLookup) EventByID(context.Context, int64) (Event, error) {
	l.calls++
	if l.err != nil {
		return Event{}, l.err
	}
	if l.event.ID == 0 {
		return Event{}, fmt.Errorf("unexpected EventByID call")
	}
	return l.event, nil
}
