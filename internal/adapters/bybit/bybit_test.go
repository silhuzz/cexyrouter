package bybit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pedro/cex-router/pkg/types"
	"github.com/shopspring/decimal"
)

func TestSignedGETRequestUsesByteIdenticalQueryFixture(t *testing.T) {
	fixture := readSigningFixture(t)
	timestampMillis, err := strconv.ParseInt(fixture.Timestamp, 10, 64)
	if err != nil {
		t.Fatalf("ParseInt(timestamp): %v", err)
	}

	adapter := New(
		WithBaseURL("https://example.test"),
		WithCredentials(fixture.APIKey, fixture.APISecret),
		WithRecvWindow(fixture.RecvWindow),
		WithReferer("bybitapinode"),
		WithNow(func() time.Time {
			return time.UnixMilli(timestampMillis)
		}),
	)

	req, err := adapter.newSignedGETRequest(context.Background(), coinInfoPath, fixture.QueryString)
	if err != nil {
		t.Fatalf("newSignedGETRequest() error = %v", err)
	}

	if req.URL.RawQuery != fixture.QueryString {
		t.Fatalf("RawQuery = %q, want byte-identical %q", req.URL.RawQuery, fixture.QueryString)
	}
	if req.URL.RequestURI() != coinInfoPath+"?"+fixture.QueryString {
		t.Fatalf("RequestURI = %q, want %q", req.URL.RequestURI(), coinInfoPath+"?"+fixture.QueryString)
	}
	if req.Header.Get("X-BAPI-API-KEY") != fixture.APIKey {
		t.Fatalf("X-BAPI-API-KEY = %q, want %q", req.Header.Get("X-BAPI-API-KEY"), fixture.APIKey)
	}
	if req.Header.Get("X-BAPI-TIMESTAMP") != fixture.Timestamp {
		t.Fatalf("X-BAPI-TIMESTAMP = %q, want %q", req.Header.Get("X-BAPI-TIMESTAMP"), fixture.Timestamp)
	}
	if req.Header.Get("X-BAPI-RECV-WINDOW") != fixture.RecvWindow {
		t.Fatalf("X-BAPI-RECV-WINDOW = %q, want %q", req.Header.Get("X-BAPI-RECV-WINDOW"), fixture.RecvWindow)
	}
	if req.Header.Get("X-BAPI-SIGN") != fixture.Signature {
		t.Fatalf("X-BAPI-SIGN = %q, want %q", req.Header.Get("X-BAPI-SIGN"), fixture.Signature)
	}
	if req.Header.Get("X-Referer") != "bybitapinode" {
		t.Fatalf("X-Referer = %q, want bybitapinode", req.Header.Get("X-Referer"))
	}
	if req.Header.Get("Referer") != "bybitapinode" {
		t.Fatalf("Referer = %q, want bybitapinode", req.Header.Get("Referer"))
	}
}

func TestFetchRailsUsesCoinInfoFixture(t *testing.T) {
	fixture := readSigningFixture(t)
	timestampMillis, err := strconv.ParseInt(fixture.Timestamp, 10, 64)
	if err != nil {
		t.Fatalf("ParseInt(timestamp): %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != coinInfoPath {
			t.Errorf("path = %q, want %q", r.URL.Path, coinInfoPath)
			http.NotFound(w, r)
			return
		}
		if r.URL.RawQuery != "" {
			t.Errorf("RawQuery = %q, want empty full snapshot query", r.URL.RawQuery)
		}
		if r.Header.Get("X-BAPI-API-KEY") != fixture.APIKey {
			t.Errorf("X-BAPI-API-KEY = %q, want %q", r.Header.Get("X-BAPI-API-KEY"), fixture.APIKey)
		}
		if r.Header.Get("X-BAPI-TIMESTAMP") != fixture.Timestamp {
			t.Errorf("X-BAPI-TIMESTAMP = %q, want %q", r.Header.Get("X-BAPI-TIMESTAMP"), fixture.Timestamp)
		}
		if r.Header.Get("X-BAPI-RECV-WINDOW") != fixture.RecvWindow {
			t.Errorf("X-BAPI-RECV-WINDOW = %q, want %q", r.Header.Get("X-BAPI-RECV-WINDOW"), fixture.RecvWindow)
		}
		if r.Header.Get("X-BAPI-SIGN") == "" {
			t.Errorf("X-BAPI-SIGN is empty")
		}

		body, err := os.ReadFile(filepath.Join("testdata", "coin_info_all.json"))
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
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials(fixture.APIKey, fixture.APISecret),
		WithRecvWindow(fixture.RecvWindow),
		WithNow(func() time.Time {
			return time.UnixMilli(timestampMillis)
		}),
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

	usdtEth := result.Snapshots[0]
	if usdtEth.ExchangeSlug != Slug {
		t.Fatalf("ExchangeSlug = %q, want %q", usdtEth.ExchangeSlug, Slug)
	}
	if usdtEth.CoinSymbol != "USDT" || usdtEth.CoinName != "Tether USD" {
		t.Fatalf("unexpected coin fields: %#v", usdtEth)
	}
	if usdtEth.RawChainSymbol != "ETH" || usdtEth.RawChainName != "Ethereum" || usdtEth.RawNetworkID != "ETH" {
		t.Fatalf("unexpected chain fields: %#v", usdtEth)
	}
	if usdtEth.ContractAddress != "0xdAC17F958D2ee523a2206206994597C13D831ec7" {
		t.Fatalf("ContractAddress = %q", usdtEth.ContractAddress)
	}
	if !usdtEth.DepositEnabled || usdtEth.WithdrawEnabled {
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
	wantObservedAt := time.UnixMilli(timestampMillis).UTC()
	if !usdtEth.ObservedAt.Equal(wantObservedAt) {
		t.Fatalf("ObservedAt = %s, want %s", usdtEth.ObservedAt, wantObservedAt)
	}

	usdtArb := result.Snapshots[2]
	if usdtArb.DepositEnabled || !usdtArb.WithdrawEnabled {
		t.Fatalf("unexpected ARB status: deposit=%v withdraw=%v", usdtArb.DepositEnabled, usdtArb.WithdrawEnabled)
	}
	assertDecimal(t, usdtArb.WithdrawFee, "0.2")
	assertDecimal(t, usdtArb.WithdrawFeePercent, "0.01")
	if usdtArb.WithdrawFeeType != types.FeeTypeHybrid {
		t.Fatalf("WithdrawFeeType = %q, want %q", usdtArb.WithdrawFeeType, types.FeeTypeHybrid)
	}

	xrp := result.Snapshots[3]
	if xrp.CoinSymbol != "XRP" || xrp.RawChainSymbol != "XRP" || xrp.RawChainName != "XRP Ledger" {
		t.Fatalf("unexpected XRP fields: %#v", xrp)
	}
	if !xrp.DepositEnabled || !xrp.WithdrawEnabled {
		t.Fatalf("unexpected XRP status: deposit=%v withdraw=%v", xrp.DepositEnabled, xrp.WithdrawEnabled)
	}
	assertDecimal(t, xrp.WithdrawMin, "20")
	assertDecimal(t, xrp.WithdrawFee, "0.25")
}

func readSigningFixture(t *testing.T) signingFixture {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", "signing.json"))
	if err != nil {
		t.Fatalf("read signing fixture: %v", err)
	}
	var fixture signingFixture
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("unmarshal signing fixture: %v", err)
	}
	return fixture
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

type signingFixture struct {
	APIKey      string `json:"api_key"`
	APISecret   string `json:"api_secret"`
	Timestamp   string `json:"timestamp"`
	RecvWindow  string `json:"recv_window"`
	QueryString string `json:"query_string"`
	Signature   string `json:"signature"`
}
