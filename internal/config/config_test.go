package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestLoadIngesterUsesPublicDefaultExchanges(t *testing.T) {
	withCleanEnv(t, map[string]string{
		"DATABASE_URL": "postgres://example",
	})

	cfg, err := LoadIngester()
	if err != nil {
		t.Fatalf("LoadIngester() error = %v", err)
	}

	want := []string{"bithumb", "bitget", "kucoin", "gate", "htx", "coinex", "whitebit", "bitmart"}
	if !reflect.DeepEqual(cfg.Exchanges, want) {
		t.Fatalf("Exchanges = %#v, want %#v", cfg.Exchanges, want)
	}
}

func TestLoadIngesterAutoAddsCredentialedMainVenuesToDefault(t *testing.T) {
	withCleanEnv(t, map[string]string{
		"DATABASE_URL":       "postgres://example",
		"BINANCE_API_KEY":    "binance-key",
		"BINANCE_API_SECRET": "binance-secret",
		"BYBIT_API_KEY":      "bybit-key",
		"BYBIT_API_SECRET":   "bybit-secret",
	})

	cfg, err := LoadIngester()
	if err != nil {
		t.Fatalf("LoadIngester() error = %v", err)
	}

	want := []string{"bithumb", "bitget", "kucoin", "gate", "htx", "coinex", "whitebit", "bitmart", "binance", "bybit"}
	if !reflect.DeepEqual(cfg.Exchanges, want) {
		t.Fatalf("Exchanges = %#v, want %#v", cfg.Exchanges, want)
	}
}

func TestLoadIngesterNormalizesGateIOAlias(t *testing.T) {
	withCleanEnv(t, map[string]string{
		"DATABASE_URL":       "postgres://example",
		"INGESTER_EXCHANGES": "gate.io, gateio, bitget",
	})

	cfg, err := LoadIngester()
	if err != nil {
		t.Fatalf("LoadIngester() error = %v", err)
	}

	want := []string{"gate", "bitget"}
	if !reflect.DeepEqual(cfg.Exchanges, want) {
		t.Fatalf("Exchanges = %#v, want %#v", cfg.Exchanges, want)
	}
}

func TestLoadIngesterExplicitBinanceRequiresCredentials(t *testing.T) {
	withCleanEnv(t, map[string]string{
		"DATABASE_URL":       "postgres://example",
		"INGESTER_EXCHANGES": "binance",
	})

	_, err := LoadIngester()
	if err == nil {
		t.Fatalf("LoadIngester() error = nil, want missing credentials")
	}
	if !strings.Contains(err.Error(), "BINANCE_API_KEY") || !strings.Contains(err.Error(), "BINANCE_API_SECRET") {
		t.Fatalf("LoadIngester() error = %v, want missing Binance credentials", err)
	}
}

func withCleanEnv(t *testing.T, values map[string]string) {
	t.Helper()

	keys := []string{
		"DATABASE_URL",
		"INGESTER_EXCHANGES",
		"BINANCE_API_KEY",
		"BINANCE_API_SECRET",
		"BYBIT_API_KEY",
		"BYBIT_API_SECRET",
		"OKX_API_KEY",
		"OKX_API_SECRET",
		"OKX_PASSPHRASE",
		"UPBIT_API_KEY",
		"UPBIT_API_SECRET",
	}

	previous := make(map[string]string, len(keys))
	present := make(map[string]bool, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			previous[key] = value
			present[key] = true
		}
		_ = os.Unsetenv(key)
	}

	for key, value := range values {
		if _, tracked := present[key]; !tracked {
			if old, ok := os.LookupEnv(key); ok {
				previous[key] = old
				present[key] = true
			}
		}
		_ = os.Setenv(key, value)
	}

	t.Cleanup(func() {
		for _, key := range keys {
			if present[key] {
				_ = os.Setenv(key, previous[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
		for key := range values {
			if contains(keys, key) {
				continue
			}
			if present[key] {
				_ = os.Setenv(key, previous[key])
			} else {
				_ = os.Unsetenv(key)
			}
		}
	})
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
