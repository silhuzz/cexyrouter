package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ExchangeLockKeyBase int64 = 900_200_000
	BotLeaderLockKey    int64 = 900_300_001
)

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}

	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.MinConns = 1
	cfg.MaxConns = 10

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pgx pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

func Health(ctx context.Context, pool *pgxpool.Pool) error {
	var one int
	if err := pool.QueryRow(ctx, "select 1").Scan(&one); err != nil {
		return err
	}
	if one != 1 {
		return fmt.Errorf("unexpected health query result: %d", one)
	}
	return nil
}

func ExchangeLockKey(exchangeID int32) int64 {
	return ExchangeLockKeyBase + int64(exchangeID)
}
