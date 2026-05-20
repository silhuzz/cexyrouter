package upbit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewJWTIncludesLiteralQueryHash(t *testing.T) {
	const (
		accessKey    = "access-key"
		secretKey    = "secret-key"
		nonce        = "11111111-2222-4333-8444-555555555555"
		literalQuery = "currency=USDT&net_type=ETH"
	)

	token, err := newJWT(accessKey, secretKey, nonce, literalQuery)
	if err != nil {
		t.Fatalf("newJWT() error = %v", err)
	}

	header, payload := decodeJWT(t, token)
	if header["alg"] != "HS256" || header["typ"] != "JWT" {
		t.Fatalf("unexpected JWT header: %#v", header)
	}
	if payload["access_key"] != accessKey || payload["nonce"] != nonce {
		t.Fatalf("unexpected JWT payload identity fields: %#v", payload)
	}
	if payload["query_hash_alg"] != "SHA512" {
		t.Fatalf("query_hash_alg = %q, want SHA512", payload["query_hash_alg"])
	}
	const wantHash = "1e514f8d1bfc680dff28df6059db9b8e6338e9d4aa11d80a4353a503954e0b6b2007d4a727a0877b8edcc0119b97c8f7537d3975cbbb356ac7d7d3edf717b95e"
	if payload["query_hash"] != wantHash {
		t.Fatalf("query_hash = %q, want %q", payload["query_hash"], wantHash)
	}
	assertHS256Signature(t, token, secretKey)
}

func TestNewJWTOmitsQueryHashWithoutQuery(t *testing.T) {
	token, err := newJWT("access-key", "secret-key", "nonce", "")
	if err != nil {
		t.Fatalf("newJWT() error = %v", err)
	}

	_, payload := decodeJWT(t, token)
	if _, ok := payload["query_hash"]; ok {
		t.Fatalf("query_hash present for empty query: %#v", payload)
	}
	if _, ok := payload["query_hash_alg"]; ok {
		t.Fatalf("query_hash_alg present for empty query: %#v", payload)
	}
}

func TestFetchRailsUsesWalletStatusFixture(t *testing.T) {
	server := fixtureServer(t, map[string]string{
		walletStatusPath: "wallet_status_all.json",
	})
	defer server.Close()

	adapter := New(
		WithBaseURL(server.URL),
		WithHTTPClient(server.Client()),
		WithCredentials("access-key", "secret-key"),
		WithNonceSource(func() (string, error) {
			return "11111111-2222-4333-8444-555555555555", nil
		}),
	)
	result, err := adapter.FetchRails(context.Background())
	if err != nil {
		t.Fatalf("FetchRails() error = %v", err)
	}

	if !result.Complete {
		t.Fatalf("Complete = false, notes = %v", result.Notes)
	}
	if len(result.Snapshots) != 5 {
		t.Fatalf("len(Snapshots) = %d, want 5", len(result.Snapshots))
	}

	usdtEth := result.Snapshots[0]
	if usdtEth.ExchangeSlug != Slug {
		t.Fatalf("ExchangeSlug = %q, want %q", usdtEth.ExchangeSlug, Slug)
	}
	if usdtEth.CoinSymbol != "USDT" {
		t.Fatalf("CoinSymbol = %q, want USDT", usdtEth.CoinSymbol)
	}
	if usdtEth.RawChainSymbol != "ETH" || usdtEth.RawChainName != "Ethereum" || usdtEth.RawNetworkID != "ETH" {
		t.Fatalf("unexpected chain fields: %#v", usdtEth)
	}
	if !usdtEth.DepositEnabled || !usdtEth.WithdrawEnabled {
		t.Fatalf("unexpected USDT ETH status: deposit=%v withdraw=%v", usdtEth.DepositEnabled, usdtEth.WithdrawEnabled)
	}
	if usdtEth.WithdrawFee != nil || usdtEth.WithdrawMin != nil || usdtEth.WithdrawFeeType != "" {
		t.Fatalf("wallet status should not synthesize fee fields: %#v", usdtEth)
	}
	if usdtEth.ObservedAt.IsZero() {
		t.Fatal("ObservedAt is zero")
	}

	usdtTrx := result.Snapshots[1]
	if usdtTrx.DepositEnabled || !usdtTrx.WithdrawEnabled {
		t.Fatalf("unexpected withdraw_only mapping: deposit=%v withdraw=%v", usdtTrx.DepositEnabled, usdtTrx.WithdrawEnabled)
	}

	usdtApt := result.Snapshots[2]
	if !usdtApt.DepositEnabled || usdtApt.WithdrawEnabled {
		t.Fatalf("unexpected deposit_only mapping: deposit=%v withdraw=%v", usdtApt.DepositEnabled, usdtApt.WithdrawEnabled)
	}

	xrp := result.Snapshots[3]
	if xrp.DepositEnabled || xrp.WithdrawEnabled {
		t.Fatalf("unexpected paused mapping: deposit=%v withdraw=%v", xrp.DepositEnabled, xrp.WithdrawEnabled)
	}

	ada := result.Snapshots[4]
	if ada.DepositEnabled || ada.WithdrawEnabled {
		t.Fatalf("unexpected unsupported mapping: deposit=%v withdraw=%v", ada.DepositEnabled, ada.WithdrawEnabled)
	}
}

func TestSnapshotsFromWalletStatusesMarksUnknownStateIncomplete(t *testing.T) {
	result, err := snapshotsFromWalletStatuses([]walletStatus{{
		Currency:    "BTC",
		WalletState: "maintenance",
		NetType:     "BTC",
		NetworkName: "Bitcoin",
	}}, fixedObservedAt())
	if err != nil {
		t.Fatalf("snapshotsFromWalletStatuses() error = %v", err)
	}
	if result.Complete {
		t.Fatal("Complete = true, want false")
	}
	if len(result.Snapshots) != 1 {
		t.Fatalf("len(Snapshots) = %d, want 1", len(result.Snapshots))
	}
	if len(result.Notes) == 0 {
		t.Fatal("Notes is empty, want unexpected wallet_state note")
	}
}

func TestFetchRailsRequiresCredentials(t *testing.T) {
	adapter := New()
	_, err := adapter.FetchRails(context.Background())
	if err == nil {
		t.Fatal("FetchRails() error = nil, want credentials error")
	}
}

func fixtureServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		if got := r.URL.RawQuery; got != "" {
			t.Errorf("RawQuery = %q, want empty", got)
		}

		fixture, ok := routes[r.URL.Path]
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

func decodeJWT(t *testing.T, token string) (map[string]string, map[string]string) {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d, want 3", len(parts))
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}

	var header map[string]string
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatalf("unmarshal JWT header: %v", err)
	}
	var payload map[string]string
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("unmarshal JWT payload: %v", err)
	}
	return header, payload
}

func assertHS256Signature(t *testing.T, token string, secretKey string) {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT parts = %d, want 3", len(parts))
	}

	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(unsigned))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatalf("signature = %q, want %q", parts[2], want)
	}
}

func fixedObservedAt() time.Time {
	return time.Unix(1_771_424_000, 0).UTC()
}
