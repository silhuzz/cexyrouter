package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	exchangeinfo "github.com/silhuzz/cexyrouter/internal/exchanges"
)

const defaultPublicExchanges = "bithumb,bitget,kucoin,gate,htx,coinex,whitebit,bitmart"

type IngesterConfig struct {
	DatabaseURL    string
	LogLevel       string
	PollInterval   time.Duration
	PollTimeout    time.Duration
	MaxConcurrency int
	RunOnce        bool
	Exchanges      []string

	BinanceAPIKey    string
	BinanceAPISecret string
	BinanceBaseURL   string
	BitgetBaseURL    string
	BitMartBaseURL   string
	BybitBaseURL     string
	BybitAPIKey      string
	BybitAPISecret   string
	BybitRecvWindow  string
	BybitReferer     string
	CoinExBaseURL    string
	GateBaseURL      string
	HTXBaseURL       string
	KuCoinBaseURL    string
	OKXBaseURL       string
	OKXAPIKey        string
	OKXAPISecret     string
	OKXPassphrase    string
	OKXSimulated     bool
	UpbitAPIKey      string
	UpbitAPISecret   string
	UpbitBaseURL     string
	WhiteBITBaseURL  string
}

type APIConfig struct {
	DatabaseURL          string
	ListenAddr           string
	CORSAllowedOrigins   []string
	RateLimitPerMin      int
	LogLevel             string
	InternalMetricsToken string
}

type BotConfig struct {
	DatabaseURL      string
	LogLevel         string
	TelegramBotToken string
}

func LoadIngester() (IngesterConfig, error) {
	exchangeValue, exchangeOverride := os.LookupEnv("INGESTER_EXCHANGES")
	if !exchangeOverride {
		exchangeValue = defaultPublicExchanges
	}

	cfg := IngesterConfig{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		LogLevel:         env("LOG_LEVEL", "info"),
		PollInterval:     durationSeconds("POLL_INTERVAL_SECONDS", 30),
		PollTimeout:      durationSeconds("POLL_TIMEOUT_SECONDS", 10),
		MaxConcurrency:   intEnv("INGESTER_MAX_CONCURRENCY", 4),
		RunOnce:          boolEnv("INGESTER_RUN_ONCE", false),
		Exchanges:        exchangeinfo.NormalizeSlugs(splitCSV(exchangeValue)),
		BinanceAPIKey:    os.Getenv("BINANCE_API_KEY"),
		BinanceAPISecret: os.Getenv("BINANCE_API_SECRET"),
		BinanceBaseURL:   os.Getenv("BINANCE_BASE_URL"),
		BitgetBaseURL:    os.Getenv("BITGET_BASE_URL"),
		BitMartBaseURL:   os.Getenv("BITMART_BASE_URL"),
		BybitBaseURL:     os.Getenv("BYBIT_BASE_URL"),
		BybitAPIKey:      os.Getenv("BYBIT_API_KEY"),
		BybitAPISecret:   os.Getenv("BYBIT_API_SECRET"),
		BybitRecvWindow:  env("BYBIT_RECV_WINDOW", "10000"),
		BybitReferer:     os.Getenv("BYBIT_REFERER"),
		CoinExBaseURL:    os.Getenv("COINEX_BASE_URL"),
		GateBaseURL:      os.Getenv("GATE_BASE_URL"),
		HTXBaseURL:       os.Getenv("HTX_BASE_URL"),
		KuCoinBaseURL:    os.Getenv("KUCOIN_BASE_URL"),
		OKXBaseURL:       os.Getenv("OKX_BASE_URL"),
		OKXAPIKey:        os.Getenv("OKX_API_KEY"),
		OKXAPISecret:     os.Getenv("OKX_API_SECRET"),
		OKXPassphrase:    os.Getenv("OKX_PASSPHRASE"),
		OKXSimulated:     boolEnv("OKX_SIMULATED_TRADING", false),
		UpbitAPIKey:      os.Getenv("UPBIT_API_KEY"),
		UpbitAPISecret:   os.Getenv("UPBIT_API_SECRET"),
		UpbitBaseURL:     env("UPBIT_BASE_URL", "https://api.upbit.com"),
		WhiteBITBaseURL:  os.Getenv("WHITEBIT_BASE_URL"),
	}
	if !exchangeOverride {
		cfg.Exchanges = appendCredentialedMainVenues(cfg.Exchanges, cfg)
	}

	var missing []string
	required(&missing, "DATABASE_URL", cfg.DatabaseURL)
	if exchangeSelected(cfg.Exchanges, "binance") {
		required(&missing, "BINANCE_API_KEY", cfg.BinanceAPIKey)
		required(&missing, "BINANCE_API_SECRET", cfg.BinanceAPISecret)
	}
	if exchangeSelected(cfg.Exchanges, "bybit") {
		required(&missing, "BYBIT_API_KEY", cfg.BybitAPIKey)
		required(&missing, "BYBIT_API_SECRET", cfg.BybitAPISecret)
	}
	if exchangeSelected(cfg.Exchanges, "okx") {
		required(&missing, "OKX_API_KEY", cfg.OKXAPIKey)
		required(&missing, "OKX_API_SECRET", cfg.OKXAPISecret)
		required(&missing, "OKX_PASSPHRASE", cfg.OKXPassphrase)
	}
	if exchangeSelected(cfg.Exchanges, "upbit") {
		required(&missing, "UPBIT_API_KEY", cfg.UpbitAPIKey)
		required(&missing, "UPBIT_API_SECRET", cfg.UpbitAPISecret)
	}
	if err := validate(missing); err != nil {
		return IngesterConfig{}, err
	}
	return cfg, nil
}

func LoadAPI() (APIConfig, error) {
	cfg := APIConfig{
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		ListenAddr:           listenAddr(),
		CORSAllowedOrigins:   splitCSV(env("CORS_ALLOWED_ORIGINS", "*")),
		RateLimitPerMin:      intEnv("RATE_LIMIT_PER_MIN", 60),
		LogLevel:             env("LOG_LEVEL", "info"),
		InternalMetricsToken: os.Getenv("INTERNAL_METRICS_TOKEN"),
	}
	var missing []string
	required(&missing, "DATABASE_URL", cfg.DatabaseURL)
	required(&missing, "LISTEN_ADDR", cfg.ListenAddr)
	if err := validate(missing); err != nil {
		return APIConfig{}, err
	}
	if err := warnIfLikelyPgBouncer(cfg.DatabaseURL); err != nil {
		return APIConfig{}, err
	}
	return cfg, nil
}

func LoadBot() (BotConfig, error) {
	cfg := BotConfig{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		LogLevel:         env("LOG_LEVEL", "info"),
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
	}
	var missing []string
	required(&missing, "DATABASE_URL", cfg.DatabaseURL)
	required(&missing, "TELEGRAM_BOT_TOKEN", cfg.TelegramBotToken)
	if err := validate(missing); err != nil {
		return BotConfig{}, err
	}
	if err := warnIfLikelyPgBouncer(cfg.DatabaseURL); err != nil {
		return BotConfig{}, err
	}
	return cfg, nil
}

func ConfigureLogger(level string) {
	var slogLevel slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel})))
}

func required(missing *[]string, name string, value string) {
	if strings.TrimSpace(value) == "" {
		*missing = append(*missing, name)
	}
}

func validate(missing []string) error {
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	return nil
}

func env(name string, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func listenAddr() string {
	if value := strings.TrimSpace(os.Getenv("LISTEN_ADDR")); value != "" {
		return value
	}
	if port := strings.TrimSpace(os.Getenv("PORT")); port != "" {
		return "0.0.0.0:" + port
	}
	return ""
}

func intEnv(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func durationSeconds(name string, fallback int) time.Duration {
	return time.Duration(intEnv(name, fallback)) * time.Second
}

func boolEnv(name string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return []string{"*"}
	}
	return out
}

func appendCredentialedMainVenues(current []string, cfg IngesterConfig) []string {
	extra := append([]string(nil), current...)
	if strings.TrimSpace(cfg.BinanceAPIKey) != "" && strings.TrimSpace(cfg.BinanceAPISecret) != "" {
		extra = append(extra, "binance")
	}
	if strings.TrimSpace(cfg.BybitAPIKey) != "" && strings.TrimSpace(cfg.BybitAPISecret) != "" {
		extra = append(extra, "bybit")
	}
	return exchangeinfo.NormalizeSlugs(extra)
}

func exchangeSelected(exchanges []string, slug string) bool {
	normalized := exchangeinfo.NormalizeSlug(slug)
	for _, exchange := range exchanges {
		if exchangeinfo.NormalizeSlug(exchange) == normalized {
			return true
		}
	}
	return false
}

func warnIfLikelyPgBouncer(databaseURL string) error {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	host := strings.ToLower(parsed.Host)
	if strings.Contains(host, "pgbouncer") || strings.Contains(host, "pooler") {
		slog.Warn("DATABASE_URL looks like a pooled connection; LISTEN requires a session-level Postgres connection")
	}
	return nil
}
