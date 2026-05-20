package ws

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const railEventsChannel = "rail_events"

type PGListener struct {
	db *pgxpool.Pool
}

func NewPGListener(db *pgxpool.Pool) *PGListener {
	return &PGListener{db: db}
}

func (l *PGListener) Listen(ctx context.Context, notifications chan<- Notification) error {
	if l == nil || l.db == nil {
		return fmt.Errorf("database pool is not configured")
	}
	if notifications == nil {
		return fmt.Errorf("notifications channel is required")
	}

	conn, err := l.db.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+railEventsChannel); err != nil {
		return err
	}
	defer func() {
		unlistenCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlistenCtx, "UNLISTEN "+railEventsChannel)
	}()

	for {
		pgNotification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		notification, err := ParseNotification([]byte(pgNotification.Payload))
		if err != nil {
			return err
		}
		select {
		case notifications <- notification:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
