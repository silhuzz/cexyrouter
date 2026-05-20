package ws

import (
	"encoding/json"
	"testing"
)

func TestFiltersMatch(t *testing.T) {
	event := Event{
		EventType: "deposit_off",
		Exchange:  ExchangeRef{Ref: Ref{Slug: "binance"}},
		Coin:      CoinRef{Ref: Ref{Slug: "usdt"}},
		Chain:     ChainRef{Ref: Ref{Slug: "tron"}},
	}

	tests := []struct {
		name    string
		filters Filters
		want    bool
	}{
		{
			name: "missing fields match any",
			filters: Filters{
				Exchange: []string{"binance"},
			},
			want: true,
		},
		{
			name: "or within field and across-field and",
			filters: Filters{
				Exchange:  []string{"upbit", "binance"},
				Coin:      []string{"usdt"},
				Chain:     []string{"tron", "solana"},
				EventType: []string{"deposit_off", "withdraw_off"},
			},
			want: true,
		},
		{
			name: "field mismatch rejects",
			filters: Filters{
				Exchange: []string{"upbit"},
			},
			want: false,
		},
		{
			name: "empty array matches nothing",
			filters: Filters{
				Exchange: []string{},
			},
			want: false,
		},
		{
			name: "event type mismatch rejects",
			filters: Filters{
				EventType: []string{"withdraw_off"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := NormalizeFilters(tt.filters)
			if err != nil {
				t.Fatalf("normalize filters: %v", err)
			}
			if got := filters.Match(event); got != tt.want {
				t.Fatalf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSubscription(t *testing.T) {
	rawCursor, err := EncodeCursor(Cursor{
		OccurredAt: testTime(),
		ID:         99,
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	payload, err := json.Marshal(map[string]any{
		"type": "subscribe",
		"filters": map[string]any{
			"exchange":   []string{" Binance ", "binance"},
			"coin":       []string{"USDT"},
			"chain":      []string{"TRON"},
			"event_type": []string{"deposit_off"},
		},
		"since": rawCursor,
	})
	if err != nil {
		t.Fatalf("marshal subscription: %v", err)
	}

	subscription, err := ParseSubscription(payload)
	if err != nil {
		t.Fatalf("parse subscription: %v", err)
	}
	if subscription.Type != "subscribe" {
		t.Fatalf("type = %q", subscription.Type)
	}
	if subscription.Since == nil || subscription.Since.ID != 99 {
		t.Fatalf("unexpected since cursor: %+v", subscription.Since)
	}
	if got := subscription.Filters.Exchange; len(got) != 1 || got[0] != "binance" {
		t.Fatalf("exchange filters = %#v", got)
	}
	if got := subscription.Filters.Coin; len(got) != 1 || got[0] != "usdt" {
		t.Fatalf("coin filters = %#v", got)
	}
}

func TestParseSubscriptionRejectsUnsupportedEventType(t *testing.T) {
	payload := []byte(`{"type":"subscribe","filters":{"event_type":["unknown"]},"since":null}`)
	if _, err := ParseSubscription(payload); err == nil {
		t.Fatalf("expected unsupported event type error")
	}
}
