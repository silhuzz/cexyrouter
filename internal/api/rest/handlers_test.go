package rest

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestRailsCursorRoundTrip(t *testing.T) {
	raw, err := encodeRailsCursor(rail{
		Exchange: exchangeRef{ID: 5},
		Coin:     coinRef{ID: 7},
		Chain:    chainRef{ID: 11},
	})
	if err != nil {
		t.Fatalf("encode rails cursor: %v", err)
	}

	cursor, err := decodeRailsCursor(raw)
	if err != nil {
		t.Fatalf("decode rails cursor: %v", err)
	}
	if cursor.ExchangeID != 5 || cursor.CoinID != 7 || cursor.ChainID != 11 {
		t.Fatalf("unexpected cursor: %+v", cursor)
	}
}

func TestEventsCursorRoundTrip(t *testing.T) {
	occurredAt := time.Date(2026, 5, 20, 12, 34, 56, 789, time.UTC)
	raw, err := encodeEventsCursor(event{
		ID:         42,
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("encode events cursor: %v", err)
	}

	cursor, err := decodeEventsCursor(raw)
	if err != nil {
		t.Fatalf("decode events cursor: %v", err)
	}
	if cursor.ID != 42 || !cursor.OccurredAt.Equal(occurredAt) {
		t.Fatalf("unexpected cursor: %+v", cursor)
	}
}

func TestParseSinceDuration(t *testing.T) {
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	since, err := parseSince("24h", now)
	if err != nil {
		t.Fatalf("parse since: %v", err)
	}
	want := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if !since.Equal(want) {
		t.Fatalf("since = %s, want %s", since, want)
	}
}

func TestParseSinceTimestamp(t *testing.T) {
	since, err := parseSince("2026-05-21T04:26:35Z", time.Now())
	if err != nil {
		t.Fatalf("parse since: %v", err)
	}
	want := time.Date(2026, 5, 21, 4, 26, 35, 0, time.UTC)
	if !since.Equal(want) {
		t.Fatalf("since = %s, want %s", since, want)
	}
}

func TestParseSinceRejectsInvalidValue(t *testing.T) {
	if _, err := parseSince("yesterday", time.Now()); err == nil {
		t.Fatal("parse since error = nil, want error")
	}
}

func TestQueryValuesAcceptsRepeatedAndCommaSeparatedValues(t *testing.T) {
	values := queryValues(map[string][]string{
		"event_type": {"deposit_off, deposit_on", "withdraw_off", "deposit_off"},
	}, "event_type")
	want := []string{"deposit_off", "deposit_on", "withdraw_off"}
	if len(values) != len(want) {
		t.Fatalf("values = %#v, want %#v", values, want)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("values = %#v, want %#v", values, want)
		}
	}
}

func TestEstimateWithdrawFee(t *testing.T) {
	tests := []struct {
		name     string
		feeType  *string
		fee      *string
		percent  *string
		amount   string
		expected string
	}{
		{
			name:     "fixed fee",
			feeType:  strPtr("fixed"),
			fee:      strPtr("1.25"),
			amount:   "100",
			expected: "1.25",
		},
		{
			name:     "percent fee",
			feeType:  strPtr("percent"),
			percent:  strPtr("0.1"),
			amount:   "1000",
			expected: "1",
		},
		{
			name:     "hybrid fee",
			feeType:  strPtr("hybrid"),
			fee:      strPtr("0.5"),
			percent:  strPtr("0.25"),
			amount:   "200",
			expected: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, err := decimal.NewFromString(tt.amount)
			if err != nil {
				t.Fatalf("parse amount: %v", err)
			}
			actual, err := estimateWithdrawFee(rail{
				WithdrawFeeType:    tt.feeType,
				WithdrawFee:        tt.fee,
				WithdrawFeePercent: tt.percent,
			}, amount, true)
			if err != nil {
				t.Fatalf("estimate fee: %v", err)
			}
			if actual.String() != tt.expected {
				t.Fatalf("fee = %s, want %s", actual.String(), tt.expected)
			}
		})
	}
}

func TestRouteFeeEstimateMarksMissingFeeMetadataUnknown(t *testing.T) {
	label, fee, known, err := routeFeeEstimate(rail{}, decimal.Zero, false)
	if err != nil {
		t.Fatalf("route fee estimate: %v", err)
	}
	if known {
		t.Fatal("fee known = true, want false")
	}
	if label != "n/a" {
		t.Fatalf("label = %q, want n/a", label)
	}
	if !fee.IsZero() {
		t.Fatalf("fee = %s, want zero sort value", fee)
	}
}

func TestRouteFeeEstimateUsesKnownFixedFeeWithoutFeeType(t *testing.T) {
	label, fee, known, err := routeFeeEstimate(rail{
		WithdrawFee: strPtr("0.52"),
	}, decimal.Zero, false)
	if err != nil {
		t.Fatalf("route fee estimate: %v", err)
	}
	if !known {
		t.Fatal("fee known = false, want true")
	}
	if label != "0.52" || fee.String() != "0.52" {
		t.Fatalf("estimate = (%q, %s), want 0.52", label, fee)
	}
}

func strPtr(value string) *string {
	return &value
}
