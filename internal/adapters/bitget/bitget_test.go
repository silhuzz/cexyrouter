package bitget

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/silhuzz/cexyrouter/pkg/types"
)

func TestFetchRailsUsesCoinListFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != coinsPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code":"00000",
			"msg":"success",
			"data":[
				{
					"coin":"USDT",
					"name":"Tether USD",
					"chains":[
						{"chain":"ERC20","withdrawable":"true","rechargeable":"true","withdrawFee":"3.5","extraWithdrawFee":"0","depositConfirm":"12","minWithdrawAmount":"10","contractAddress":"0xdAC17F958D2ee523a2206206994597C13D831ec7"},
						{"chain":"TRC20","withdrawable":"false","rechargeable":"true","withdrawFee":"1","extraWithdrawFee":"0.01","depositConfirm":"20","minWithdrawAmount":"5"}
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
	if erc20.ExchangeSlug != Slug || erc20.CoinSymbol != "USDT" || erc20.RawChainSymbol != "ERC20" {
		t.Fatalf("unexpected ERC20 snapshot: %#v", erc20)
	}
	if !erc20.DepositEnabled || !erc20.WithdrawEnabled {
		t.Fatalf("unexpected ERC20 status: deposit=%v withdraw=%v", erc20.DepositEnabled, erc20.WithdrawEnabled)
	}
	if erc20.WithdrawFeeType != types.FeeTypeFixed {
		t.Fatalf("WithdrawFeeType = %q, want %q", erc20.WithdrawFeeType, types.FeeTypeFixed)
	}
	if erc20.ContractAddress != "0xdAC17F958D2ee523a2206206994597C13D831ec7" {
		t.Fatalf("ContractAddress = %q", erc20.ContractAddress)
	}

	trc20 := result.Snapshots[1]
	if !trc20.DepositEnabled || trc20.WithdrawEnabled {
		t.Fatalf("unexpected TRC20 status: deposit=%v withdraw=%v", trc20.DepositEnabled, trc20.WithdrawEnabled)
	}
	if trc20.WithdrawFeeType != types.FeeTypeHybrid {
		t.Fatalf("WithdrawFeeType = %q, want %q", trc20.WithdrawFeeType, types.FeeTypeHybrid)
	}
}
