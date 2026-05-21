package okx

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

var fixedNow = time.Date(2026, 5, 20, 14, 30, 1, 234000000, time.UTC)

func TestSignUsesOKXPrehashWithQuery(t *testing.T) {
	vector := readSigningVector(t)

	got := sign(vector.Secret, vector.Timestamp, vector.Method, vector.RequestPath, []byte(vector.Body))
	if got != vector.ExpectedSignature {
		t.Fatalf("signature = %q, want %q", got, vector.ExpectedSignature)
	}
}

func TestFetchRailsUsesFixturesAndSignsRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != currenciesPath {
			http.NotFound(w, r)
			return
		}

		assertHeader(t, r, "OK-ACCESS-KEY", "test-key")
		assertHeader(t, r, "OK-ACCESS-PASSPHRASE", "test-passphrase")
		assertHeader(t, r, "OK-ACCESS-TIMESTAMP", "2026-05-20T14:30:01.234Z")
		assertHeader(t, r, "OK-ACCESS-SIGN", "doQOIhE/3vrcIGnhjdcq7l7ZwYtXoA0tNEvdBiH+BrI=")

		body := readFixture(t, "currencies_all.json")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	adapter := New(
		WithCredentials("test-key", "test-secret", "test-passphrase"),
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithNow(func() time.Time { return fixedNow }),
	)

	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}

	assertCompleteSnapshots(t, result)
}

func TestSnapshotsFromCurrenciesMarksIncompleteForSkippedRows(t *testing.T) {
	records := []currencyRecord{
		{
			Currency:   "USDT",
			Chain:      "USDT-ERC20",
			CanDeposit: true,
		},
		{
			Currency: "USDC",
		},
	}

	result, err := snapshotsFromCurrencies(records, fixedNow)
	if err != nil {
		t.Fatalf("snapshotsFromCurrencies() error = %v", err)
	}
	if result.Complete {
		t.Fatal("Complete = true, want false")
	}
	if len(result.Snapshots) != 1 {
		t.Fatalf("len(Snapshots) = %d, want 1", len(result.Snapshots))
	}
	if len(result.Notes) == 0 {
		t.Fatal("Notes is empty, want skipped row note")
	}
}

func assertCompleteSnapshots(t *testing.T, result types.FetchResult) {
	t.Helper()

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
	if usdtEth.CoinSymbol != "USDT" || usdtEth.CoinName != "Tether" {
		t.Fatalf("unexpected coin fields: %#v", usdtEth)
	}
	if usdtEth.RawChainSymbol != "ERC20" || usdtEth.RawChainName != "ERC20" || usdtEth.RawNetworkID != "USDT-ERC20" {
		t.Fatalf("unexpected chain fields: %#v", usdtEth)
	}
	if usdtEth.ContractAddress != "0xdAC17F958D2ee523a2206206994597C13D831ec7" {
		t.Fatalf("ContractAddress = %q", usdtEth.ContractAddress)
	}
	if !usdtEth.DepositEnabled || !usdtEth.WithdrawEnabled {
		t.Fatalf("unexpected USDT ERC20 status: deposit=%v withdraw=%v", usdtEth.DepositEnabled, usdtEth.WithdrawEnabled)
	}
	assertInt(t, usdtEth.DepositConfirmations, 12)
	assertDecimal(t, usdtEth.WithdrawMin, "10")
	assertDecimal(t, usdtEth.WithdrawFee, "3.5")
	if usdtEth.WithdrawFeeType != types.FeeTypeFixed {
		t.Fatalf("WithdrawFeeType = %q, want %q", usdtEth.WithdrawFeeType, types.FeeTypeFixed)
	}
	if usdtEth.ObservedAt.IsZero() {
		t.Fatal("ObservedAt is zero")
	}

	usdtTron := result.Snapshots[1]
	if usdtTron.RawChainSymbol != "TRC20" || usdtTron.RawChainName != "TRC20" || usdtTron.RawNetworkID != "USDT-TRC20" {
		t.Fatalf("unexpected TRC20 chain fields: %#v", usdtTron)
	}
	if !usdtTron.DepositEnabled || usdtTron.WithdrawEnabled {
		t.Fatalf("unexpected TRC20 status: deposit=%v withdraw=%v", usdtTron.DepositEnabled, usdtTron.WithdrawEnabled)
	}
	assertDecimal(t, usdtTron.WithdrawMin, "5")
	assertDecimal(t, usdtTron.WithdrawFee, "1")

	ethArb := result.Snapshots[2]
	if ethArb.RawChainSymbol != "Arbitrum One" || ethArb.RawChainName != "Arbitrum One" || ethArb.RawNetworkID != "ETH-Arbitrum One" {
		t.Fatalf("unexpected Arbitrum chain fields: %#v", ethArb)
	}
	if ethArb.DepositEnabled || !ethArb.WithdrawEnabled {
		t.Fatalf("unexpected Arbitrum status: deposit=%v withdraw=%v", ethArb.DepositEnabled, ethArb.WithdrawEnabled)
	}
	if ethArb.WithdrawMin != nil {
		t.Fatalf("WithdrawMin = %v, want nil", ethArb.WithdrawMin)
	}
	assertDecimal(t, ethArb.WithdrawFee, "0.0001")
	assertInt(t, ethArb.DepositConfirmations, 64)
}

func assertHeader(t *testing.T, r *http.Request, key string, want string) {
	t.Helper()

	if got := r.Header.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
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

func readSigningVector(t *testing.T) signingVector {
	t.Helper()

	var vector signingVector
	if err := json.Unmarshal(readFixture(t, "signing_vector.json"), &vector); err != nil {
		t.Fatalf("unmarshal signing vector: %v", err)
	}
	return vector
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

type signingVector struct {
	Secret            string `json:"secret"`
	Timestamp         string `json:"timestamp"`
	Method            string `json:"method"`
	RequestPath       string `json:"request_path"`
	Body              string `json:"body"`
	ExpectedSignature string `json:"expected_signature"`
}
