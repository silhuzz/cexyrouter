package gate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchRailsUsesCurrenciesFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != currenciesPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"currency":"USDT",
				"name":"Tether",
				"delisted":false,
				"withdraw_disabled":false,
				"withdraw_delayed":false,
				"deposit_disabled":false,
				"chain":"ETH",
				"chains":[
					{"name":"ETH","withdraw_disabled":false,"withdraw_delayed":false,"deposit_disabled":false},
					{"name":"TRX","withdraw_disabled":true,"withdraw_delayed":false,"deposit_disabled":false}
				]
			},
			{
				"currency":"BTC_BTC",
				"name":"Bitcoin",
				"delisted":false,
				"withdraw_disabled":false,
				"withdraw_delayed":true,
				"deposit_disabled":true,
				"chain":"BTC"
			}
		]`))
	}))
	defer server.Close()

	observedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	adapter := New(WithBaseURL(server.URL), WithHTTPClient(server.Client()), WithNow(func() time.Time { return observedAt }))

	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}
	if !result.Complete {
		t.Fatalf("Complete = false, notes = %v", result.Notes)
	}
	if len(result.Snapshots) != 3 {
		t.Fatalf("len(Snapshots) = %d, want 3", len(result.Snapshots))
	}

	eth := result.Snapshots[0]
	if eth.ExchangeSlug != Slug || eth.CoinSymbol != "USDT" || eth.RawChainSymbol != "ETH" {
		t.Fatalf("unexpected ETH snapshot: %#v", eth)
	}
	if !eth.DepositEnabled || !eth.WithdrawEnabled {
		t.Fatalf("unexpected ETH status: deposit=%v withdraw=%v", eth.DepositEnabled, eth.WithdrawEnabled)
	}

	trx := result.Snapshots[1]
	if !trx.DepositEnabled || trx.WithdrawEnabled {
		t.Fatalf("unexpected TRX status: deposit=%v withdraw=%v", trx.DepositEnabled, trx.WithdrawEnabled)
	}

	btc := result.Snapshots[2]
	if btc.CoinSymbol != "BTC" || btc.DepositEnabled || btc.WithdrawEnabled {
		t.Fatalf("unexpected BTC snapshot: %#v", btc)
	}
}
