package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/silhuzz/cexyrouter/internal/adapters/binance"
	"github.com/silhuzz/cexyrouter/internal/adapters/bitget"
	"github.com/silhuzz/cexyrouter/internal/adapters/bithumb"
	"github.com/silhuzz/cexyrouter/internal/adapters/bitmart"
	"github.com/silhuzz/cexyrouter/internal/adapters/bybit"
	"github.com/silhuzz/cexyrouter/internal/adapters/coinex"
	"github.com/silhuzz/cexyrouter/internal/adapters/gate"
	"github.com/silhuzz/cexyrouter/internal/adapters/htx"
	"github.com/silhuzz/cexyrouter/internal/adapters/kucoin"
	"github.com/silhuzz/cexyrouter/internal/adapters/okx"
	"github.com/silhuzz/cexyrouter/internal/adapters/upbit"
	"github.com/silhuzz/cexyrouter/internal/adapters/whitebit"
	"github.com/silhuzz/cexyrouter/internal/envfile"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

func main() {
	envPath := flag.String("env", ".env", "env file to load if present")
	exchanges := flag.String("exchanges", "bithumb,bitget,kucoin,gate,htx,coinex,whitebit,bitmart", "comma-separated exchanges or all")
	timeout := flag.Duration("timeout", 20*time.Second, "timeout per adapter")
	flag.Parse()

	if err := envfile.Load(*envPath, false); err != nil {
		fmt.Fprintf(os.Stderr, "load env: %v\n", err)
		os.Exit(1)
	}

	client := &http.Client{Timeout: *timeout}
	adapters, skipped := selectedAdapters(*exchanges, client)
	for _, msg := range skipped {
		fmt.Println("skip:", msg)
	}
	if len(adapters) == 0 {
		fmt.Println("no adapters selected with available credentials")
		os.Exit(1)
	}

	okCount := 0
	for _, adapter := range adapters {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		start := time.Now()
		result, err := adapter.FetchRails(ctx)
		cancel()
		elapsed := time.Since(start).Round(time.Millisecond)
		if err != nil {
			fmt.Printf("%-8s error after %s: %v\n", adapter.Slug(), elapsed, err)
			continue
		}
		okCount++
		fmt.Printf("%-8s rails=%d complete=%t notes=%d elapsed=%s\n", adapter.Slug(), len(result.Snapshots), result.Complete, len(result.Notes), elapsed)
		for _, sample := range sampleSnapshots(result.Snapshots, 3) {
			fmt.Printf("         %s %s deposit=%t withdraw=%t fee_type=%s\n", sample.CoinSymbol, sample.RawChainSymbol, sample.DepositEnabled, sample.WithdrawEnabled, sample.WithdrawFeeType)
		}
		if len(result.Notes) > 0 {
			fmt.Printf("         first_note=%s\n", result.Notes[0])
		}
	}
	if okCount != len(adapters) {
		os.Exit(1)
	}
}

func selectedAdapters(raw string, client *http.Client) ([]types.Adapter, []string) {
	wanted := parseWanted(raw)
	var adapters []types.Adapter
	var skipped []string

	add := func(slug string, adapter types.Adapter, missing ...string) {
		if !wanted[slug] {
			return
		}
		var absent []string
		for _, key := range missing {
			if strings.TrimSpace(os.Getenv(key)) == "" {
				absent = append(absent, key)
			}
		}
		if len(absent) > 0 {
			skipped = append(skipped, fmt.Sprintf("%s missing %s", slug, strings.Join(absent, ",")))
			return
		}
		adapters = append(adapters, adapter)
	}

	add("binance",
		binance.New(os.Getenv("BINANCE_API_KEY"), os.Getenv("BINANCE_API_SECRET"), binance.WithBaseURL(os.Getenv("BINANCE_BASE_URL")), binance.WithHTTPClient(client)),
		"BINANCE_API_KEY", "BINANCE_API_SECRET",
	)
	add("bitget", bitget.New(bitget.WithBaseURL(os.Getenv("BITGET_BASE_URL")), bitget.WithHTTPClient(client)))
	add("bitmart", bitmart.New(bitmart.WithBaseURL(os.Getenv("BITMART_BASE_URL")), bitmart.WithHTTPClient(client)))
	add("bybit",
		bybit.New(bybit.WithCredentials(os.Getenv("BYBIT_API_KEY"), os.Getenv("BYBIT_API_SECRET")), bybit.WithBaseURL(os.Getenv("BYBIT_BASE_URL")), bybit.WithRecvWindow(envDefault("BYBIT_RECV_WINDOW", "10000")), bybit.WithReferer(os.Getenv("BYBIT_REFERER")), bybit.WithHTTPClient(client)),
		"BYBIT_API_KEY", "BYBIT_API_SECRET",
	)
	add("coinex", coinex.New(coinex.WithBaseURL(os.Getenv("COINEX_BASE_URL")), coinex.WithHTTPClient(client)))
	add("gate", gate.New(gate.WithBaseURL(os.Getenv("GATE_BASE_URL")), gate.WithHTTPClient(client)))
	add("htx", htx.New(htx.WithBaseURL(os.Getenv("HTX_BASE_URL")), htx.WithHTTPClient(client)))
	add("kucoin", kucoin.New(kucoin.WithBaseURL(os.Getenv("KUCOIN_BASE_URL")), kucoin.WithHTTPClient(client)))
	add("okx",
		okx.New(okx.WithCredentials(os.Getenv("OKX_API_KEY"), os.Getenv("OKX_API_SECRET"), os.Getenv("OKX_PASSPHRASE")), okx.WithBaseURL(os.Getenv("OKX_BASE_URL")), okx.WithSimulatedTrading(boolEnv("OKX_SIMULATED_TRADING", false)), okx.WithHTTPClient(client)),
		"OKX_API_KEY", "OKX_API_SECRET", "OKX_PASSPHRASE",
	)
	add("bithumb", bithumb.New(bithumb.WithHTTPClient(client)))
	add("upbit",
		upbit.New(upbit.WithCredentials(os.Getenv("UPBIT_API_KEY"), os.Getenv("UPBIT_API_SECRET")), upbit.WithHTTPClient(client), upbit.WithBaseURL(envDefault("UPBIT_BASE_URL", "https://api.upbit.com"))),
		"UPBIT_API_KEY", "UPBIT_API_SECRET",
	)
	add("whitebit", whitebit.New(whitebit.WithBaseURL(os.Getenv("WHITEBIT_BASE_URL")), whitebit.WithHTTPClient(client)))

	sort.Slice(adapters, func(i, j int) bool { return adapters[i].Slug() < adapters[j].Slug() })
	return adapters, skipped
}

func parseWanted(raw string) map[string]bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	all := map[string]bool{
		"binance":  true,
		"bitget":   true,
		"bitmart":  true,
		"bithumb":  true,
		"bybit":    true,
		"coinex":   true,
		"gate":     true,
		"htx":      true,
		"kucoin":   true,
		"okx":      true,
		"upbit":    true,
		"whitebit": true,
	}
	if raw == "" || raw == "all" {
		return all
	}
	wanted := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		if part != "" {
			wanted[part] = true
		}
	}
	return wanted
}

func sampleSnapshots(snapshots []types.RailSnapshot, limit int) []types.RailSnapshot {
	if len(snapshots) <= limit {
		return snapshots
	}
	return snapshots[:limit]
}

func envDefault(key string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func boolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
