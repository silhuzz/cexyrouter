package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	"github.com/silhuzz/cexyrouter/internal/config"
	"github.com/silhuzz/cexyrouter/internal/db"
	"github.com/silhuzz/cexyrouter/internal/envfile"
	"github.com/silhuzz/cexyrouter/internal/ingester"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := envfile.Load(envFilePath(), false); err != nil {
		slog.Error("load env file", "error", err)
		os.Exit(1)
	}

	cfg, err := config.LoadIngester()
	if err != nil {
		slog.Error("load ingester config", "error", err)
		os.Exit(1)
	}
	config.ConfigureLogger(cfg.LogLevel)

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	adapters := buildAdapters(cfg)
	if len(adapters) == 0 {
		slog.Error("no ingester adapters configured")
		os.Exit(1)
	}

	runner := &ingester.Runner{
		DB:             pool,
		Logger:         slog.Default(),
		Timeout:        cfg.PollTimeout,
		MaxConcurrency: cfg.MaxConcurrency,
	}

	runOnce := func() bool {
		results, err := runner.RunOnce(ctx, adapters)
		for _, result := range results {
			if result.Error != nil {
				slog.Error("ingester adapter failed",
					"exchange", result.ExchangeSlug,
					"error", result.Error,
				)
				continue
			}
			slog.Info("ingester cycle complete",
				"exchange", result.ExchangeSlug,
				"fetched", result.Fetched,
				"normalized", result.Normalized,
				"complete", result.Complete,
				"mutations", result.Mutations,
				"events", result.Events,
				"notes", len(result.Notes),
				"fetch_elapsed_ms", result.FetchElapsed.Milliseconds(),
				"elapsed_ms", result.Elapsed.Milliseconds(),
			)
		}
		if err != nil {
			slog.Error("ingester cycle completed with adapter errors", "error", err)
			return false
		}
		return true
	}

	if cfg.RunOnce {
		if !runOnce() {
			os.Exit(1)
		}
		return
	}

	slog.Info("ingester polling", "poll_interval", cfg.PollInterval.String(), "max_concurrency", cfg.MaxConcurrency, "exchanges", cfg.Exchanges)
	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		runOnce()
		select {
		case <-ctx.Done():
			slog.Info("ingester stopped")
			return
		case <-ticker.C:
		}
	}
}

func envFilePath() string {
	if path := strings.TrimSpace(os.Getenv("ENV_FILE")); path != "" {
		return path
	}
	return ".env"
}

func buildAdapters(cfg config.IngesterConfig) []types.Adapter {
	client := &http.Client{Timeout: cfg.PollTimeout}
	selected := make(map[string]bool, len(cfg.Exchanges))
	for _, exchange := range cfg.Exchanges {
		selected[strings.ToLower(strings.TrimSpace(exchange))] = true
	}

	var adapters []types.Adapter
	if selected["binance"] {
		adapters = append(adapters, binance.New(cfg.BinanceAPIKey, cfg.BinanceAPISecret, binance.WithBaseURL(cfg.BinanceBaseURL), binance.WithHTTPClient(client)))
	}
	if selected["bitget"] {
		adapters = append(adapters, bitget.New(bitget.WithBaseURL(cfg.BitgetBaseURL), bitget.WithHTTPClient(client)))
	}
	if selected["bitmart"] {
		adapters = append(adapters, bitmart.New(bitmart.WithBaseURL(cfg.BitMartBaseURL), bitmart.WithHTTPClient(client)))
	}
	if selected["bybit"] {
		adapters = append(adapters, bybit.New(bybit.WithCredentials(cfg.BybitAPIKey, cfg.BybitAPISecret), bybit.WithBaseURL(cfg.BybitBaseURL), bybit.WithRecvWindow(cfg.BybitRecvWindow), bybit.WithReferer(cfg.BybitReferer), bybit.WithHTTPClient(client)))
	}
	if selected["coinex"] {
		adapters = append(adapters, coinex.New(coinex.WithBaseURL(cfg.CoinExBaseURL), coinex.WithHTTPClient(client)))
	}
	if selected["gate"] {
		adapters = append(adapters, gate.New(gate.WithBaseURL(cfg.GateBaseURL), gate.WithHTTPClient(client)))
	}
	if selected["htx"] {
		adapters = append(adapters, htx.New(htx.WithBaseURL(cfg.HTXBaseURL), htx.WithHTTPClient(client)))
	}
	if selected["kucoin"] {
		adapters = append(adapters, kucoin.New(kucoin.WithBaseURL(cfg.KuCoinBaseURL), kucoin.WithHTTPClient(client)))
	}
	if selected["okx"] {
		adapters = append(adapters, okx.New(okx.WithCredentials(cfg.OKXAPIKey, cfg.OKXAPISecret, cfg.OKXPassphrase), okx.WithBaseURL(cfg.OKXBaseURL), okx.WithSimulatedTrading(cfg.OKXSimulated), okx.WithHTTPClient(client)))
	}
	if selected["bithumb"] {
		adapters = append(adapters, bithumb.New(bithumb.WithHTTPClient(client)))
	}
	if selected["upbit"] {
		adapters = append(adapters, upbit.New(upbit.WithCredentials(cfg.UpbitAPIKey, cfg.UpbitAPISecret), upbit.WithBaseURL(cfg.UpbitBaseURL), upbit.WithHTTPClient(client)))
	}
	if selected["whitebit"] {
		adapters = append(adapters, whitebit.New(whitebit.WithBaseURL(cfg.WhiteBITBaseURL), whitebit.WithHTTPClient(client)))
	}
	return adapters
}
