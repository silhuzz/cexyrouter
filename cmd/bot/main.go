package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/pedro/cex-router/internal/config"
	"github.com/pedro/cex-router/internal/db"
	"github.com/pedro/cex-router/internal/envfile"
	"github.com/pedro/cex-router/internal/tg"
	"github.com/pedro/cex-router/internal/tg/commands"
	"github.com/pedro/cex-router/internal/tg/dispatch"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := envfile.Load(envFilePath(), false); err != nil {
		slog.Error("load env file", "error", err)
		os.Exit(1)
	}

	cfg, err := config.LoadBot()
	if err != nil {
		slog.Error("load bot config", "error", err)
		os.Exit(1)
	}
	config.ConfigureLogger(cfg.LogLevel)

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	client := tg.NewTelegramClient(cfg.TelegramBotToken, nil, slog.Default())
	repo := commands.NewSQLRepository(pool)
	bot := tg.NewBot(client, repo, client, slog.Default())
	dispatcher := &dispatch.Runner{DB: pool, Sender: client, Logger: slog.Default()}
	go func() {
		if err := dispatcher.Run(ctx); err != nil && ctx.Err() == nil {
			slog.Error("telegram dispatcher stopped unexpectedly", "error", err)
			stop()
		}
	}()

	slog.Info("bot listening for telegram commands")
	if err := bot.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("bot stopped unexpectedly", "error", err)
		os.Exit(1)
	}
	slog.Info("bot stopped")
}

func envFilePath() string {
	if path := strings.TrimSpace(os.Getenv("ENV_FILE")); path != "" {
		return path
	}
	return ".env"
}
