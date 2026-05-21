package kucoin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/silhuzz/cexyrouter/pkg/types"
)

func TestFetchRailsUsesCurrenciesFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != currenciesPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code":"200000",
			"data":[
				{
					"currency":"ETH",
					"name":"ETH",
					"fullName":"Ethereum",
					"items":[
						{"chainName":"ERC20","minWithdrawSize":"0.003","minDepositSize":"0.00025","withdrawFeeRate":"0","minWithdrawFee":"0.0015","isWithdrawEnabled":true,"isDepositEnabled":true,"confirms":96,"chainId":"eth","contractAddress":"0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2"},
						{"chainName":"Arbitrum","minWithdrawSize":"0.001","withdrawFeeRate":"0.01","minWithdrawFee":"0.0001","isWithdrawEnabled":false,"isDepositEnabled":true,"confirms":"12","chainId":"arb"}
					]
				}
			]
		}`))
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
	if len(result.Snapshots) != 2 {
		t.Fatalf("len(Snapshots) = %d, want 2", len(result.Snapshots))
	}

	erc20 := result.Snapshots[0]
	if erc20.ExchangeSlug != Slug || erc20.CoinSymbol != "ETH" || erc20.RawChainSymbol != "ERC20" {
		t.Fatalf("unexpected ERC20 snapshot: %#v", erc20)
	}
	if !erc20.DepositEnabled || !erc20.WithdrawEnabled {
		t.Fatalf("unexpected ERC20 status: deposit=%v withdraw=%v", erc20.DepositEnabled, erc20.WithdrawEnabled)
	}
	if erc20.WithdrawFeeType != types.FeeTypeFixed {
		t.Fatalf("WithdrawFeeType = %q, want %q", erc20.WithdrawFeeType, types.FeeTypeFixed)
	}
	if erc20.ContractAddress != "0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2" {
		t.Fatalf("ContractAddress = %q", erc20.ContractAddress)
	}

	arb := result.Snapshots[1]
	if !arb.DepositEnabled || arb.WithdrawEnabled {
		t.Fatalf("unexpected Arbitrum status: deposit=%v withdraw=%v", arb.DepositEnabled, arb.WithdrawEnabled)
	}
	if arb.WithdrawFeeType != types.FeeTypeHybrid {
		t.Fatalf("WithdrawFeeType = %q, want %q", arb.WithdrawFeeType, types.FeeTypeHybrid)
	}
}
