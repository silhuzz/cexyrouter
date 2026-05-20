package htx

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
		if r.URL.Path != "/v2/reference/currencies" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("authorizedUser"); got != "false" {
			t.Fatalf("authorizedUser = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"data": [{
				"currency": "usdt",
				"instStatus": "normal",
				"chains": [{
					"chain": "arc20usdt",
					"displayName": "ARBITRUM",
					"fullName": "Arbitrum One",
					"baseChain": "ARB",
					"baseChainProtocol": "ARC20",
					"depositStatus": "allowed",
					"withdrawStatus": "prohibited",
					"minWithdrawAmt": "1",
					"transactFeeWithdraw": "1",
					"numOfConfirmations": 64
				}, {
					"chain": "usdterc20",
					"displayName": "ERC20",
					"fullName": "Ethereum",
					"baseChain": "ETH",
					"baseChainProtocol": "ERC20",
					"depositStatus": "allowed",
					"withdrawStatus": "allowed",
					"minWithdrawAmt": "1",
					"transactFeeWithdraw": "2.12062",
					"numOfConfirmations": 64
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
	if arb.ExchangeSlug != Slug || arb.CoinSymbol != "USDT" || arb.RawChainSymbol != "ARB" || arb.RawNetworkID != "arc20usdt" {
		t.Fatalf("unexpected arb snapshot: %+v", arb)
	}
	if !arb.DepositEnabled || arb.WithdrawEnabled {
		t.Fatalf("unexpected arb availability: deposit=%t withdraw=%t", arb.DepositEnabled, arb.WithdrawEnabled)
	}
	if arb.DepositConfirmations == nil || *arb.DepositConfirmations != 64 {
		t.Fatalf("unexpected confirmations: %v", arb.DepositConfirmations)
	}
	if arb.WithdrawMin == nil || arb.WithdrawMin.String() != "1" {
		t.Fatalf("unexpected withdraw min: %v", arb.WithdrawMin)
	}
	if arb.WithdrawFee == nil || arb.WithdrawFee.String() != "1" || arb.WithdrawFeeType != "fixed" {
		t.Fatalf("unexpected withdraw fee: %v type=%s", arb.WithdrawFee, arb.WithdrawFeeType)
	}

	eth := result.Snapshots[1]
	if eth.RawChainSymbol != "ETH" || eth.RawChainName != "Ethereum" || !eth.DepositEnabled || !eth.WithdrawEnabled {
		t.Fatalf("unexpected eth snapshot: %+v", eth)
	}
	if !eth.ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt = %s", eth.ObservedAt)
	}
}

func TestFetchRailsMapsWrappedBTC(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"data": [{
				"currency": "btc",
				"instStatus": "normal",
				"chains": [{
					"chain": "btc",
					"displayName": "BTC",
					"fullName": "Bitcoin",
					"depositStatus": "allowed",
					"withdrawStatus": "allowed"
				}, {
					"chain": "wbtc",
					"displayName": "ERC20",
					"fullName": "WBTC",
					"baseChain": "ETH",
					"depositStatus": "allowed",
					"withdrawStatus": "allowed"
				}, {
					"chain": "basecbbtc",
					"displayName": "BASE",
					"fullName": "cbBTC",
					"baseChain": "BASE",
					"depositStatus": "allowed",
					"withdrawStatus": "prohibited"
				}]
			}]
		}`))
	}))
	defer server.Close()

	adapter := New(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}
	if len(result.Snapshots) != 3 {
		t.Fatalf("snapshots len = %d", len(result.Snapshots))
	}
	if got := result.Snapshots[0].CoinSymbol; got != "BTC" {
		t.Fatalf("native symbol = %q", got)
	}
	if got := result.Snapshots[1].CoinSymbol; got != "WBTC" {
		t.Fatalf("wbtc symbol = %q", got)
	}
	if got := result.Snapshots[2].CoinSymbol; got != "CBBTC" {
		t.Fatalf("cbbtc symbol = %q", got)
	}
}
