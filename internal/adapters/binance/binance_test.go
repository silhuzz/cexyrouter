package binance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

func TestSignQuery(t *testing.T) {
	query := "timestamp=1700000000123&recvWindow=10000"
	got := signQuery(query, "test-secret")
	want := "dca8a80fba2e4d01760c39607750eab8c2611bab70fb4b25ea8b2df469077ff3"
	if got != want {
		t.Fatalf("signQuery() = %q, want %q", got, want)
	}
}

func TestFetchRailsUsesSignedFixture(t *testing.T) {
	fixedNow := time.Unix(1700000000, 123000000).UTC()
	expectedQuery := "timestamp=1700000000123&recvWindow=10000&signature=dca8a80fba2e4d01760c39607750eab8c2611bab70fb4b25ea8b2df469077ff3"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != capitalConfigPath {
			t.Errorf("path = %q, want %q", r.URL.Path, capitalConfigPath)
			http.NotFound(w, r)
			return
		}
		if r.URL.RawQuery != expectedQuery {
			t.Errorf("RawQuery = %q, want %q", r.URL.RawQuery, expectedQuery)
			http.Error(w, "bad query", http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("X-MBX-APIKEY"); got != "test-key" {
			t.Errorf("X-MBX-APIKEY = %q, want test-key", got)
			http.Error(w, "bad api key", http.StatusUnauthorized)
			return
		}

		body, err := os.ReadFile(filepath.Join("testdata", "capital_config_getall.json"))
		if err != nil {
			t.Errorf("read fixture: %v", err)
			http.Error(w, "fixture error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := New(
		"test-key",
		"test-secret",
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithClock(func() time.Time { return fixedNow }),
		WithRecvWindow(10*time.Second),
		WithLimiter(noopLimiter{}),
	)
	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}
	if !result.Complete {
		t.Fatalf("Complete = false, notes = %v", result.Notes)
	}
	if len(result.Snapshots) != 4 {
		t.Fatalf("len(Snapshots) = %d, want 4", len(result.Snapshots))
	}
}

func TestSnapshotsFromAssetsUsesFixture(t *testing.T) {
	var assets []capitalAsset
	body, err := os.ReadFile(filepath.Join("testdata", "capital_config_getall.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := json.Unmarshal(body, &assets); err != nil {
		t.Fatalf("json.Unmarshal(): %v", err)
	}

	observedAt := time.Date(2026, 5, 20, 7, 0, 0, 0, time.UTC)
	result, err := snapshotsFromAssets(assets, observedAt)
	if err != nil {
		t.Fatalf("snapshotsFromAssets() error = %v", err)
	}
	if !result.Complete {
		t.Fatalf("Complete = false, notes = %v", result.Notes)
	}
	if len(result.Snapshots) != 4 {
		t.Fatalf("len(Snapshots) = %d, want 4", len(result.Snapshots))
	}

	usdtEth := result.Snapshots[0]
	if usdtEth.ExchangeSlug != Slug {
		t.Fatalf("ExchangeSlug = %q, want %q", usdtEth.ExchangeSlug, Slug)
	}
	if usdtEth.CoinSymbol != "USDT" || usdtEth.CoinName != "TetherUS" {
		t.Fatalf("unexpected coin fields: %#v", usdtEth)
	}
	if usdtEth.RawChainSymbol != "ETH" || usdtEth.RawChainName != "Ethereum (ERC20)" || usdtEth.RawNetworkID != "ETH" {
		t.Fatalf("unexpected chain fields: %#v", usdtEth)
	}
	if !usdtEth.DepositEnabled || !usdtEth.WithdrawEnabled {
		t.Fatalf("unexpected ETH status: deposit=%v withdraw=%v", usdtEth.DepositEnabled, usdtEth.WithdrawEnabled)
	}
	assertInt(t, usdtEth.DepositConfirmations, 12)
	assertDecimal(t, usdtEth.WithdrawMin, "10")
	assertDecimal(t, usdtEth.WithdrawFee, "3.5")
	if usdtEth.WithdrawFeeType != types.FeeTypeFixed {
		t.Fatalf("WithdrawFeeType = %q, want %q", usdtEth.WithdrawFeeType, types.FeeTypeFixed)
	}
	if usdtEth.WithdrawFeePercent != nil {
		t.Fatalf("WithdrawFeePercent = %v, want nil", usdtEth.WithdrawFeePercent)
	}
	if !usdtEth.ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt = %s, want %s", usdtEth.ObservedAt, observedAt)
	}

	usdtTrx := result.Snapshots[1]
	if usdtTrx.RawChainSymbol != "TRX" || usdtTrx.RawChainName != "Tron (TRC20)" || usdtTrx.RawNetworkID != "TRX" {
		t.Fatalf("unexpected TRX chain fields: %#v", usdtTrx)
	}
	if !usdtTrx.DepositEnabled || usdtTrx.WithdrawEnabled {
		t.Fatalf("unexpected TRX status: deposit=%v withdraw=%v", usdtTrx.DepositEnabled, usdtTrx.WithdrawEnabled)
	}
	assertInt(t, usdtTrx.DepositConfirmations, 1)
	assertDecimal(t, usdtTrx.WithdrawMin, "5")
	assertDecimal(t, usdtTrx.WithdrawFee, "1")

	usdtBSC := result.Snapshots[2]
	if usdtBSC.DepositEnabled || !usdtBSC.WithdrawEnabled {
		t.Fatalf("unexpected BSC status: deposit=%v withdraw=%v", usdtBSC.DepositEnabled, usdtBSC.WithdrawEnabled)
	}
	assertDecimal(t, usdtBSC.WithdrawMin, "0.8")
	assertDecimal(t, usdtBSC.WithdrawFee, "0.29")

	xrp := result.Snapshots[3]
	if xrp.DepositEnabled || !xrp.WithdrawEnabled {
		t.Fatalf("unexpected XRP status: deposit=%v withdraw=%v", xrp.DepositEnabled, xrp.WithdrawEnabled)
	}
	assertInt(t, xrp.DepositConfirmations, 1)
	assertDecimal(t, xrp.WithdrawMin, "20")
	assertDecimal(t, xrp.WithdrawFee, "0.2")
}

func assertDecimal(t *testing.T, actual *decimal.Decimal, want string) {
	t.Helper()

	if actual == nil {
		t.Fatalf("decimal = nil, want %s", want)
	}
	expected, err := decimal.NewFromString(want)
	if err != nil {
		t.Fatalf("decimal.NewFromString(%q): %v", want, err)
	}
	if !actual.Equal(expected) {
		t.Fatalf("decimal = %s, want %s", actual.String(), want)
	}
}

func assertInt(t *testing.T, actual *int, want int) {
	t.Helper()

	if actual == nil {
		t.Fatalf("int = nil, want %d", want)
	}
	if *actual != want {
		t.Fatalf("int = %d, want %d", *actual, want)
	}
}
