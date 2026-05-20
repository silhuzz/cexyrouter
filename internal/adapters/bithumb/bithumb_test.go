package bithumb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/pedro/cex-router/pkg/types"
	"github.com/shopspring/decimal"
)

func TestFetchRailsUsesFixtures(t *testing.T) {
	server := fixtureServer(t, map[string]string{
		feePath:           "fees_all.json",
		networkStatusPath: "network_status_all.json",
	})
	defer server.Close()

	adapter := New(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
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

	usdtEth := result.Snapshots[0]
	if usdtEth.ExchangeSlug != Slug {
		t.Fatalf("ExchangeSlug = %q, want %q", usdtEth.ExchangeSlug, Slug)
	}
	if usdtEth.CoinSymbol != "USDT" || usdtEth.CoinName != "테더" {
		t.Fatalf("unexpected coin fields: %#v", usdtEth)
	}
	if usdtEth.RawChainSymbol != "ETH" || usdtEth.RawChainName != "Ethereum" || usdtEth.RawNetworkID != "ETH" {
		t.Fatalf("unexpected chain fields: %#v", usdtEth)
	}
	if !usdtEth.DepositEnabled || usdtEth.WithdrawEnabled {
		t.Fatalf("unexpected USDT status: deposit=%v withdraw=%v", usdtEth.DepositEnabled, usdtEth.WithdrawEnabled)
	}
	assertDecimal(t, usdtEth.WithdrawMin, "10")
	assertDecimal(t, usdtEth.WithdrawFee, "3.5")
	if usdtEth.WithdrawFeeType != types.FeeTypeFixed {
		t.Fatalf("WithdrawFeeType = %q, want %q", usdtEth.WithdrawFeeType, types.FeeTypeFixed)
	}
	if usdtEth.WithdrawFeePercent != nil {
		t.Fatalf("WithdrawFeePercent = %v, want nil", usdtEth.WithdrawFeePercent)
	}
	if usdtEth.ObservedAt.IsZero() {
		t.Fatal("ObservedAt is zero")
	}

	usdtTron := result.Snapshots[1]
	if usdtTron.RawChainSymbol != "TRX" || usdtTron.RawChainName != "Tron" || usdtTron.RawNetworkID != "TRX" {
		t.Fatalf("unexpected Tron chain fields: %#v", usdtTron)
	}
	if !usdtTron.DepositEnabled || !usdtTron.WithdrawEnabled {
		t.Fatalf("unexpected Tron status: deposit=%v withdraw=%v", usdtTron.DepositEnabled, usdtTron.WithdrawEnabled)
	}
	assertDecimal(t, usdtTron.WithdrawMin, "5")
	if usdtTron.WithdrawFee != nil {
		t.Fatalf("WithdrawFee = %v, want nil", usdtTron.WithdrawFee)
	}
	assertDecimal(t, usdtTron.WithdrawFeePercent, "0.01")
	if usdtTron.WithdrawFeeType != types.FeeTypePercent {
		t.Fatalf("WithdrawFeeType = %q, want %q", usdtTron.WithdrawFeeType, types.FeeTypePercent)
	}

	xrp := result.Snapshots[2]
	if !xrp.DepositEnabled || xrp.WithdrawEnabled {
		t.Fatalf("unexpected XRP status: deposit=%v withdraw=%v", xrp.DepositEnabled, xrp.WithdrawEnabled)
	}
	assertDecimal(t, xrp.WithdrawMin, "1")
	assertDecimal(t, xrp.WithdrawFee, "0.4")
}

func TestFetchRailsMarksIncompleteWhenStatusMissing(t *testing.T) {
	server := fixtureServer(t, map[string]string{
		feePath:           "fees_all.json",
		networkStatusPath: "network_status_missing.json",
	})
	defer server.Close()

	adapter := New(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}

	if result.Complete {
		t.Fatal("Complete = true, want false")
	}
	if len(result.Snapshots) != 3 {
		t.Fatalf("len(Snapshots) = %d, want 3", len(result.Snapshots))
	}
	if len(result.Notes) == 0 {
		t.Fatal("Notes is empty, want missing status note")
	}
}

func TestFetchRailsReturnsErrorForNonOKStatusEnvelope(t *testing.T) {
	server := fixtureServer(t, map[string]string{
		feePath:           "fees_all.json",
		networkStatusPath: "network_status_error.json",
	})
	defer server.Close()

	adapter := New(WithBaseURL(server.URL), WithHTTPClient(server.Client()))
	_, err := adapter.FetchRails(context.Background())
	if err == nil {
		t.Fatal("FetchRails() error = nil, want error")
	}
}

func fixtureServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fixture, ok := routes[r.URL.RequestURI()]
		if !ok {
			http.NotFound(w, r)
			return
		}

		body, err := os.ReadFile(filepath.Join("testdata", fixture))
		if err != nil {
			t.Errorf("read fixture %s: %v", fixture, err)
			http.Error(w, "fixture error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
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
