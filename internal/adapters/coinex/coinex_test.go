package coinex

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
		if r.URL.Path != "/v2/assets/all-deposit-withdraw-config" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "OK",
			"data": [{
				"asset": {"ccy": "USDC", "deposit_enabled": true, "withdraw_enabled": true},
				"chains": [{
					"chain": "ARBITRUM",
					"min_deposit_amount": "3.3",
					"min_withdraw_amount": "3.3",
					"deposit_enabled": true,
					"withdraw_enabled": true,
					"safe_confirmations": 100,
					"withdrawal_fee": "3.3"
				}, {
					"chain": "ERC20",
					"min_deposit_amount": "0.2",
					"min_withdraw_amount": "0.6",
					"deposit_enabled": true,
					"withdraw_enabled": false,
					"safe_confirmations": 12,
					"withdrawal_fee": "0.092"
				}]
			}]
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

	arb := result.Snapshots[0]
	if arb.ExchangeSlug != Slug || arb.CoinSymbol != "USDC" || arb.RawChainSymbol != "ARBITRUM" {
		t.Fatalf("unexpected arb snapshot: %+v", arb)
	}
	if !arb.DepositEnabled || !arb.WithdrawEnabled {
		t.Fatalf("unexpected arb availability: deposit=%t withdraw=%t", arb.DepositEnabled, arb.WithdrawEnabled)
	}
	if arb.WithdrawFee == nil || arb.WithdrawFee.String() != "3.3" || arb.WithdrawFeeType != "fixed" {
		t.Fatalf("unexpected withdraw fee: %v type=%s", arb.WithdrawFee, arb.WithdrawFeeType)
	}
	if arb.DepositConfirmations == nil || *arb.DepositConfirmations != 100 {
		t.Fatalf("unexpected confirmations: %v", arb.DepositConfirmations)
	}

	eth := result.Snapshots[1]
	if eth.WithdrawEnabled {
		t.Fatalf("eth withdraw should be disabled: %+v", eth)
	}
	if !eth.ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt = %s", eth.ObservedAt)
	}
}
