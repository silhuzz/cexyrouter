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

func strPtr(value string) *string {
	return &value
}
