package ws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	exchangeinfo "github.com/silhuzz/cexyrouter/internal/exchanges"
)

var validEventTypes = map[string]struct{}{
	"deposit_off":   {},
	"deposit_on":    {},
	"withdraw_off":  {},
	"withdraw_on":   {},
	"fee_changed":   {},
	"min_changed":   {},
	"rail_delisted": {},
	"rail_relisted": {},
}

type Filters struct {
	Exchange  []string `json:"exchange,omitempty"`
	Coin      []string `json:"coin,omitempty"`
	Chain     []string `json:"chain,omitempty"`
	EventType []string `json:"event_type,omitempty"`
}

type Subscription struct {
	Type    string  `json:"type"`
	Filters Filters `json:"filters"`
	Since   *Cursor `json:"since,omitempty"`
}

type subscriptionWire struct {
	Type    string          `json:"type"`
	Filters *Filters        `json:"filters"`
	Since   json.RawMessage `json:"since"`
}

func ParseSubscription(payload []byte) (Subscription, error) {
	var wire subscriptionWire
	if err := json.Unmarshal(payload, &wire); err != nil {
		return Subscription{}, fmt.Errorf("parse subscription: %w", err)
	}
	if strings.TrimSpace(wire.Type) != "subscribe" {
		return Subscription{}, fmt.Errorf("subscription type must be subscribe")
	}

	filters := Filters{}
	if wire.Filters != nil {
		normalized, err := NormalizeFilters(*wire.Filters)
		if err != nil {
			return Subscription{}, err
		}
		filters = normalized
	}

	var since *Cursor
	if len(wire.Since) > 0 && !bytes.Equal(bytes.TrimSpace(wire.Since), []byte("null")) {
		var raw string
		if err := json.Unmarshal(wire.Since, &raw); err != nil {
			return Subscription{}, fmt.Errorf("since must be a cursor string or null")
		}
		cursor, err := DecodeCursor(strings.TrimSpace(raw))
		if err != nil {
			return Subscription{}, fmt.Errorf("invalid since cursor: %w", err)
		}
		since = &cursor
	}

	return Subscription{
		Type:    "subscribe",
		Filters: filters,
		Since:   since,
	}, nil
}

func NormalizeFilters(filters Filters) (Filters, error) {
	var err error
	if filters.Exchange, err = normalizeExchangeList(filters.Exchange); err != nil {
		return Filters{}, err
	}
	if filters.Coin, err = normalizeSlugList(filters.Coin, "coin"); err != nil {
		return Filters{}, err
	}
	if filters.Chain, err = normalizeSlugList(filters.Chain, "chain"); err != nil {
		return Filters{}, err
	}
	if filters.EventType, err = normalizeEventTypes(filters.EventType); err != nil {
		return Filters{}, err
	}
	return filters, nil
}

func normalizeExchangeList(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	values, err := normalizeSlugList(values, "exchange")
	if err != nil {
		return nil, err
	}
	return exchangeinfo.NormalizeSlugs(values), nil
}

func (filters Filters) Match(event Event) bool {
	return matchField(filters.Exchange, event.Exchange.Slug) &&
		matchField(filters.Coin, event.Coin.Slug) &&
		matchField(filters.Chain, event.Chain.Slug) &&
		matchField(filters.EventType, event.EventType)
}

func (filters Filters) matchNothing() bool {
	return filters.Exchange != nil && len(filters.Exchange) == 0 ||
		filters.Coin != nil && len(filters.Coin) == 0 ||
		filters.Chain != nil && len(filters.Chain) == 0 ||
		filters.EventType != nil && len(filters.EventType) == 0
}

func normalizeSlugList(values []string, field string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			return nil, fmt.Errorf("%s filter contains an empty value", field)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeEventTypes(values []string) ([]string, error) {
	values, err := normalizeSlugList(values, "event_type")
	if err != nil {
		return nil, err
	}
	for _, value := range values {
		if _, ok := validEventTypes[value]; !ok {
			return nil, fmt.Errorf("event_type filter contains unsupported value %q", value)
		}
	}
	return values, nil
}

func matchField(allowed []string, actual string) bool {
	if allowed == nil {
		return true
	}
	if len(allowed) == 0 {
		return false
	}
	actual = strings.ToLower(strings.TrimSpace(actual))
	for _, value := range allowed {
		if value == actual {
			return true
		}
	}
	return false
}
