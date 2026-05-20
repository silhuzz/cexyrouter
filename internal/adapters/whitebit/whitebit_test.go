package whitebit

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
		if r.URL.Path != "/api/v4/public/assets" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"USDT": {
				"name": "Tether",
				"can_withdraw": true,
				"can_deposit": true,
				"min_withdraw": "6",
				"networks": {
					"deposits": ["ERC20", "TRC20"],
					"withdraws": ["TRC20"],
					"default": "ERC20"
				},
				"confirmations": {"ERC20": 32, "TRC20": 20},
				"limits": {"withdraw": {"TRC20": {"min": "1"}}}
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

	erc20 := result.Snapshots[0]
	if erc20.ExchangeSlug != Slug || erc20.CoinSymbol != "USDT" || erc20.RawChainSymbol != "ERC20" {
		t.Fatalf("unexpected erc20 snapshot: %+v", erc20)
	}
	if !erc20.DepositEnabled || erc20.WithdrawEnabled {
		t.Fatalf("unexpected erc20 availability: deposit=%t withdraw=%t", erc20.DepositEnabled, erc20.WithdrawEnabled)
	}
	if erc20.WithdrawMin == nil || erc20.WithdrawMin.String() != "6" {
		t.Fatalf("unexpected erc20 withdraw min: %v", erc20.WithdrawMin)
	}

	trc20 := result.Snapshots[1]
	if !trc20.DepositEnabled || !trc20.WithdrawEnabled {
		t.Fatalf("unexpected trc20 availability: deposit=%t withdraw=%t", trc20.DepositEnabled, trc20.WithdrawEnabled)
	}
	if trc20.WithdrawMin == nil || trc20.WithdrawMin.String() != "1" {
		t.Fatalf("unexpected trc20 withdraw min: %v", trc20.WithdrawMin)
	}
	if trc20.DepositConfirmations == nil || *trc20.DepositConfirmations != 20 {
		t.Fatalf("unexpected confirmations: %v", trc20.DepositConfirmations)
	}
}
