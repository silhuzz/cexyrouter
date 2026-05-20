package bitmart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchRails(t *testing.T) {
	observedAt := time.Date(2026, 5, 21, 9, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/account/v1/currencies" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"message": "OK",
			"code": 1000,
			"data": {
				"currencies": [{
					"currency": "USDT-ETH",
					"name": "Tether USD-ETH",
					"network": "ETH",
					"withdraw_enabled": true,
					"deposit_enabled": true,
					"withdraw_minsize": "5.005011",
					"withdraw_fee": "3.1"
				}, {
					"currency": "ZRO-ARBITRUM",
					"name": "LayerZero-ARBITRUM",
					"network": "ARBITRUM",
					"withdraw_enabled": false,
					"deposit_enabled": true,
					"withdraw_minsize": "1",
					"withdraw_fee": "0.5"
				}]
			}
		}`))
	}))
	defer server.Close()

	adapter := New(WithBaseURL(server.URL), WithHTTPClient(server.Client()), WithNow(func() time.Time { return observedAt }))
	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}
	if !result.Complete {
		t.Fatalf("Complete = false, notes=%v", result.Notes)
	}
	if len(result.Snapshots) != 2 {
		t.Fatalf("snapshots len = %d", len(result.Snapshots))
	}

	usdt := result.Snapshots[0]
	if usdt.ExchangeSlug != Slug || usdt.CoinSymbol != "USDT" || usdt.CoinName != "Tether USD" || usdt.RawChainSymbol != "ETH" {
		t.Fatalf("unexpected usdt snapshot: %+v", usdt)
	}
	if !usdt.DepositEnabled || !usdt.WithdrawEnabled {
		t.Fatalf("unexpected usdt availability: deposit=%t withdraw=%t", usdt.DepositEnabled, usdt.WithdrawEnabled)
	}
	if usdt.WithdrawFee == nil || usdt.WithdrawFee.String() != "3.1" || usdt.WithdrawFeeType != "fixed" {
		t.Fatalf("unexpected withdraw fee: %v type=%s", usdt.WithdrawFee, usdt.WithdrawFeeType)
	}

	zro := result.Snapshots[1]
	if zro.CoinSymbol != "ZRO" || zro.WithdrawEnabled {
		t.Fatalf("unexpected zro snapshot: %+v", zro)
	}
	if !zro.ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt = %s", zro.ObservedAt)
	}
}
