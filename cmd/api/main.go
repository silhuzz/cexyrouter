package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/pedro/cex-router/internal/api"
	"github.com/pedro/cex-router/internal/api/admin"
	"github.com/pedro/cex-router/internal/api/rest"
	"github.com/pedro/cex-router/internal/api/web"
	"github.com/pedro/cex-router/internal/api/ws"
	"github.com/pedro/cex-router/internal/config"
	"github.com/pedro/cex-router/internal/db"
	"github.com/pedro/cex-router/internal/envfile"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := envfile.Load(envFilePath(), false); err != nil {
		slog.Error("load env file", "error", err)
		os.Exit(1)
	}

	cfg, err := config.LoadAPI()
	if err != nil {
		slog.Error("load api config", "error", err)
		os.Exit(1)
	}
	config.ConfigureLogger(cfg.LogLevel)

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      api.NewRouter(api.Deps{Context: ctx, DB: pool, Config: cfg}, rest.Mount, admin.Mount, ws.Mount, web.Mount),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("api listening", "addr", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("api shutdown failed", "error", err)
	}
}

func envFilePath() string {
	if path := strings.TrimSpace(os.Getenv("ENV_FILE")); path != "" {
		return path
	}
	return ".env"
}
