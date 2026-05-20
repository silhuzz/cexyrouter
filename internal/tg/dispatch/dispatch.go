package dispatch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pedro/cex-router/internal/db"
	"github.com/pedro/cex-router/internal/tg/commands"
)

const (
	defaultBatchSize = 10
	maxAttempts      = 5
)

type Runner struct {
	DB      *pgxpool.Pool
	Sender  commands.Replier
	Logger  *slog.Logger
	Batch   int
	Now     func() time.Time
	Backoff func(attempts int) time.Duration
}

type Job struct {
	ID       int64
	ChatID   int64
	Body     string
	Attempts int
}

func (r *Runner) Run(ctx context.Context) error {
	if r == nil || r.DB == nil {
		return fmt.Errorf("telegram dispatcher database pool is not configured")
	}
	if r.Sender == nil {
		return fmt.Errorf("telegram dispatcher sender is not configured")
	}
	logger := r.logger()

	if err := r.ResetInFlight(ctx); err != nil {
		return err
	}

	leader, err := r.tryLeader(ctx)
	if err != nil {
		return err
	}
	if !leader {
		logger.Info("telegram dispatcher standby; another replica holds the leader lock")
		return r.waitForLeadership(ctx)
	}
	logger.Info("telegram dispatcher leader acquired")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		if err := r.Tick(ctx); err != nil {
			logger.Warn("telegram dispatcher tick failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) Tick(ctx context.Context) error {
	if _, err := r.EnqueueUndispatchedEvents(ctx, 100); err != nil {
		return err
	}
	return r.ProcessDue(ctx)
}

func (r *Runner) EnqueueUndispatchedEvents(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.DB.Query(ctx, `
		SELECT re.id
		FROM rail_events re
		WHERE EXISTS (
			SELECT 1
			FROM alert_rules ar
			JOIN tg_users u ON u.id = ar.tg_user_id
			WHERE re.event_type = ANY(ar.event_types)
			  AND re.occurred_at >= ar.created_at
			  AND (ar.exchange_id IS NULL OR ar.exchange_id = re.exchange_id)
			  AND (ar.coin_id IS NULL OR ar.coin_id = re.coin_id)
			  AND (ar.chain_id IS NULL OR ar.chain_id = re.chain_id)
			  AND NOT EXISTS (
				SELECT 1
				FROM notification_jobs nj
				WHERE nj.event_id = re.id
				  AND nj.tg_chat_id = u.tg_chat_id
			  )
		)
		ORDER BY re.occurred_at ASC, re.id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	enqueued := 0
	for rows.Next() {
		var eventID int64
		if err := rows.Scan(&eventID); err != nil {
			return enqueued, err
		}
		count, err := r.EnqueueEvent(ctx, eventID)
		if err != nil {
			return enqueued, err
		}
		enqueued += count
	}
	if err := rows.Err(); err != nil {
		return enqueued, err
	}
	return enqueued, nil
}

func (r *Runner) EnqueueEvent(ctx context.Context, eventID int64) (int, error) {
	tag, err := r.DB.Exec(ctx, `
		INSERT INTO notification_jobs (event_id, event_occurred_at, tg_chat_id, body)
		SELECT re.id,
		       re.occurred_at,
		       u.tg_chat_id,
		       format(
		         'CEX Router: %s %s %s on %s/%s',
		         re.event_type,
		         c.symbol,
		         ch.slug,
		         e.slug,
		         ch.slug
		       )
		FROM rail_events re
		JOIN exchanges e ON e.id = re.exchange_id
		JOIN coins c ON c.id = re.coin_id
		JOIN chains ch ON ch.id = re.chain_id
		JOIN alert_rules ar ON re.event_type = ANY(ar.event_types)
		 AND re.occurred_at >= ar.created_at
		 AND (ar.exchange_id IS NULL OR ar.exchange_id = re.exchange_id)
		 AND (ar.coin_id IS NULL OR ar.coin_id = re.coin_id)
		 AND (ar.chain_id IS NULL OR ar.chain_id = re.chain_id)
		JOIN tg_users u ON u.id = ar.tg_user_id
		WHERE re.id = $1
		ON CONFLICT (event_id, tg_chat_id) DO NOTHING
	`, eventID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (r *Runner) ProcessDue(ctx context.Context) error {
	jobs, err := r.claimDue(ctx, r.batchSize())
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := r.Sender.Reply(ctx, job.ChatID, job.Body); err != nil {
			if markErr := r.markFailed(ctx, job, err); markErr != nil {
				return markErr
			}
			continue
		}
		if err := r.markSent(ctx, job.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) ResetInFlight(ctx context.Context) error {
	_, err := r.DB.Exec(ctx, `
		UPDATE notification_jobs
		SET status = 'pending',
		    next_attempt_at = now()
		WHERE status = 'in_flight'
	`)
	return err
}

func (r *Runner) claimDue(ctx context.Context, limit int) ([]Job, error) {
	rows, err := r.DB.Query(ctx, `
		WITH picked AS (
			SELECT id
			FROM notification_jobs
			WHERE status IN ('pending','in_flight')
			  AND next_attempt_at <= now()
			ORDER BY next_attempt_at ASC, id ASC
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE notification_jobs nj
		SET status = 'in_flight',
		    attempts = nj.attempts + 1
		FROM picked
		WHERE nj.id = picked.id
		RETURNING nj.id, nj.tg_chat_id, nj.body, nj.attempts
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]Job, 0, limit)
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.ChatID, &job.Body, &job.Attempts); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *Runner) markSent(ctx context.Context, jobID int64) error {
	_, err := r.DB.Exec(ctx, `
		UPDATE notification_jobs
		SET status = 'sent',
		    sent_at = now(),
		    last_error = NULL
		WHERE id = $1
	`, jobID)
	return err
}

func (r *Runner) markFailed(ctx context.Context, job Job, cause error) error {
	status := "pending"
	if job.Attempts >= maxAttempts {
		status = "dead"
	}
	_, err := r.DB.Exec(ctx, `
		UPDATE notification_jobs
		SET status = $2,
		    last_error = $3,
		    next_attempt_at = now() + make_interval(secs => $4::int)
		WHERE id = $1
	`, job.ID, status, cause.Error(), int(r.backoff(job.Attempts).Seconds()))
	return err
}

func (r *Runner) tryLeader(ctx context.Context) (bool, error) {
	var ok bool
	err := r.DB.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, db.BotLeaderLockKey).Scan(&ok)
	return ok, err
}

func (r *Runner) waitForLeadership(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			leader, err := r.tryLeader(ctx)
			if err != nil {
				return err
			}
			if leader {
				return r.Run(ctx)
			}
		}
	}
}

func (r *Runner) batchSize() int {
	if r.Batch > 0 {
		return r.Batch
	}
	return defaultBatchSize
}

func (r *Runner) backoff(attempts int) time.Duration {
	if r.Backoff != nil {
		return r.Backoff(attempts)
	}
	if attempts <= 0 {
		attempts = 1
	}
	delay := time.Duration(attempts*attempts) * time.Second
	if delay > time.Minute {
		return time.Minute
	}
	return delay
}

func (r *Runner) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
