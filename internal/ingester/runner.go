package ingester

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pedro/cex-router/internal/diff"
	"github.com/pedro/cex-router/internal/normalizer"
	"github.com/pedro/cex-router/pkg/types"
	"github.com/shopspring/decimal"
)

type Runner struct {
	DB             *pgxpool.Pool
	Engine         *diff.Engine
	Logger         *slog.Logger
	Timeout        time.Duration
	MaxConcurrency int

	engineMu   sync.Mutex
	runAdapter func(context.Context, types.Adapter) (CycleResult, error)
}

type CycleResult struct {
	ExchangeSlug string
	Fetched      int
	Normalized   int
	Complete     bool
	Mutations    int
	Events       int
	Notes        []string
	Error        error
	FetchElapsed time.Duration
	Elapsed      time.Duration
}

type dbtx interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (r *Runner) RunOnce(ctx context.Context, adapters []types.Adapter) ([]CycleResult, error) {
	results := make([]CycleResult, len(adapters))
	if len(adapters) == 0 {
		return results, nil
	}

	maxConcurrency := r.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	if maxConcurrency > len(adapters) {
		maxConcurrency = len(adapters)
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for i, adapter := range adapters {
		i := i
		adapter := adapter
		wg.Add(1)
		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[i] = CycleResult{
					ExchangeSlug: adapterSlug(adapter),
					Error:        ctx.Err(),
				}
				return
			}

			result, err := r.runOneAdapter(ctx, adapter)
			if err != nil {
				result.ExchangeSlug = adapterSlug(adapter)
				result.Error = err
			}
			results[i] = result
		}()
	}
	wg.Wait()

	var errs []error
	for _, result := range results {
		if result.Error != nil {
			errs = append(errs, result.Error)
		}
	}
	return results, errors.Join(errs...)
}

func (r *Runner) runOneAdapter(ctx context.Context, adapter types.Adapter) (CycleResult, error) {
	if r.runAdapter != nil {
		return r.runAdapter(ctx, adapter)
	}
	return r.RunAdapter(ctx, adapter)
}

func adapterSlug(adapter types.Adapter) string {
	if adapter == nil {
		return ""
	}
	return adapter.Slug()
}

func (r *Runner) RunAdapter(ctx context.Context, adapter types.Adapter) (CycleResult, error) {
	started := time.Now()
	if r.DB == nil {
		return CycleResult{}, fmt.Errorf("ingester database pool is nil")
	}
	if adapter == nil {
		return CycleResult{}, fmt.Errorf("ingester adapter is nil")
	}

	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	slug := adapter.Slug()
	fetchCtx, cancel := context.WithTimeout(ctx, timeout)
	fetchStarted := time.Now()
	fetchResult, fetchErr := adapter.FetchRails(fetchCtx)
	fetchElapsed := time.Since(fetchStarted)
	cancel()
	if fetchErr != nil {
		if err := r.recordFailure(ctx, slug, fetchErr); err != nil {
			logger.Warn("record adapter failure", "exchange", slug, "error", err)
		}
		return CycleResult{
			ExchangeSlug: slug,
			FetchElapsed: fetchElapsed,
			Elapsed:      time.Since(started),
		}, fmt.Errorf("%s fetch rails: %w", slug, fetchErr)
	}

	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return CycleResult{}, fmt.Errorf("%s begin transaction: %w", slug, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	store := normalizer.NewSQLStore(tx)
	exchangeID, err := store.ExchangeID(ctx, slug)
	if err != nil {
		return CycleResult{}, fmt.Errorf("%s exchange lookup: %w", slug, err)
	}

	resolver := normalizer.NewPollResolver(store)
	normalized, err := resolver.Normalize(ctx, fetchResult)
	if err != nil {
		return CycleResult{}, fmt.Errorf("%s normalize: %w", slug, err)
	}
	normalized.Rails = mergeDuplicateRails(normalized.Rails)

	previous, err := loadPreviousRails(ctx, tx, exchangeID)
	if err != nil {
		return CycleResult{}, fmt.Errorf("%s load previous rails: %w", slug, err)
	}

	poll := diff.Poll{
		Observations: normalizedObservations(normalized.Rails),
		Complete:     normalized.Complete,
		ObservedAt:   time.Now().UTC(),
	}
	diffResult, err := r.diff(previous, poll)
	if err != nil {
		return CycleResult{}, fmt.Errorf("%s diff: %w", slug, err)
	}

	writer := sqlWriter{tx: tx}
	if err := diff.Apply(ctx, writer, diffResult); err != nil {
		return CycleResult{}, fmt.Errorf("%s apply diff: %w", slug, err)
	}

	if err := recordSuccess(ctx, tx, exchangeID); err != nil {
		return CycleResult{}, fmt.Errorf("%s record freshness: %w", slug, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CycleResult{}, fmt.Errorf("%s commit: %w", slug, err)
	}

	return CycleResult{
		ExchangeSlug: slug,
		Fetched:      len(fetchResult.Snapshots),
		Normalized:   len(normalized.Rails),
		Complete:     normalized.Complete,
		Mutations:    len(diffResult.Rails),
		Events:       len(diffResult.Events),
		Notes:        normalized.Notes,
		FetchElapsed: fetchElapsed,
		Elapsed:      time.Since(started),
	}, nil
}

func (r *Runner) diff(previous []diff.Rail, poll diff.Poll) (diff.Result, error) {
	r.engineMu.Lock()
	defer r.engineMu.Unlock()

	engine := r.Engine
	if engine == nil {
		engine = diff.NewEngine(diff.Options{})
		r.Engine = engine
	}
	return engine.Diff(previous, poll)
}

func (r *Runner) recordFailure(ctx context.Context, slug string, cause error) error {
	var exchangeID int64
	if err := r.DB.QueryRow(ctx, `SELECT id FROM exchanges WHERE slug = $1`, slug).Scan(&exchangeID); err != nil {
		return err
	}
	_, err := r.DB.Exec(ctx, `
		INSERT INTO adapter_freshness (exchange_id, last_attempt, last_error, consecutive_failures)
		VALUES ($1, now(), $2, 1)
		ON CONFLICT (exchange_id) DO UPDATE
		SET last_attempt = EXCLUDED.last_attempt,
		    last_error = EXCLUDED.last_error,
		    consecutive_failures = adapter_freshness.consecutive_failures + 1
	`, exchangeID, cause.Error())
	return err
}

func recordSuccess(ctx context.Context, tx pgx.Tx, exchangeID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO adapter_freshness (exchange_id, last_successful_poll, last_attempt, last_error, consecutive_failures)
		VALUES ($1, now(), now(), NULL, 0)
		ON CONFLICT (exchange_id) DO UPDATE
		SET last_successful_poll = EXCLUDED.last_successful_poll,
		    last_attempt = EXCLUDED.last_attempt,
		    last_error = NULL,
		    consecutive_failures = 0
	`, exchangeID)
	return err
}

func normalizedObservations(rails []normalizer.NormalizedRail) []diff.Observation {
	observations := make([]diff.Observation, 0, len(rails))
	for _, rail := range rails {
		snapshot := rail.Snapshot
		observedAt := snapshot.ObservedAt
		if observedAt.IsZero() {
			observedAt = time.Now().UTC()
		}
		observations = append(observations, diff.Observation{
			Key: diff.Key{
				ExchangeID: rail.ExchangeID,
				CoinID:     rail.CoinID,
				ChainID:    rail.ChainID,
			},
			DepositEnabled:       snapshot.DepositEnabled,
			WithdrawEnabled:      snapshot.WithdrawEnabled,
			DepositConfirmations: snapshot.DepositConfirmations,
			WithdrawMin:          snapshot.WithdrawMin,
			WithdrawFee:          snapshot.WithdrawFee,
			WithdrawFeeType:      snapshot.WithdrawFeeType,
			WithdrawFeePercent:   snapshot.WithdrawFeePercent,
			ContractAddress:      strings.TrimSpace(snapshot.ContractAddress),
			ObservedAt:           observedAt,
		})
	}
	return observations
}

func mergeDuplicateRails(rails []normalizer.NormalizedRail) []normalizer.NormalizedRail {
	merged := make(map[diff.Key]normalizer.NormalizedRail, len(rails))
	order := make([]diff.Key, 0, len(rails))
	for _, rail := range rails {
		key := diff.Key{
			ExchangeID: rail.ExchangeID,
			CoinID:     rail.CoinID,
			ChainID:    rail.ChainID,
		}
		if existing, ok := merged[key]; ok {
			merged[key] = mergeRail(existing, rail)
			continue
		}
		merged[key] = rail
		order = append(order, key)
	}

	result := make([]normalizer.NormalizedRail, 0, len(order))
	for _, key := range order {
		result = append(result, merged[key])
	}
	return result
}

func mergeRail(a normalizer.NormalizedRail, b normalizer.NormalizedRail) normalizer.NormalizedRail {
	result := a
	snapshot := result.Snapshot
	incoming := b.Snapshot

	snapshot.DepositEnabled = snapshot.DepositEnabled || incoming.DepositEnabled
	snapshot.WithdrawEnabled = snapshot.WithdrawEnabled || incoming.WithdrawEnabled
	snapshot.DepositConfirmations = minIntPtr(snapshot.DepositConfirmations, incoming.DepositConfirmations)
	snapshot.WithdrawMin = minDecimalPtr(snapshot.WithdrawMin, incoming.WithdrawMin)
	snapshot.WithdrawFee = minDecimalPtr(snapshot.WithdrawFee, incoming.WithdrawFee)
	snapshot.WithdrawFeePercent = minDecimalPtr(snapshot.WithdrawFeePercent, incoming.WithdrawFeePercent)
	snapshot.WithdrawFeeType = mergeFeeType(snapshot.WithdrawFeeType, incoming.WithdrawFeeType)
	if strings.TrimSpace(snapshot.ContractAddress) == "" && strings.TrimSpace(incoming.ContractAddress) != "" {
		snapshot.ContractAddress = incoming.ContractAddress
	}
	if incoming.ObservedAt.After(snapshot.ObservedAt) {
		snapshot.ObservedAt = incoming.ObservedAt
	}

	result.Snapshot = snapshot
	return result
}

func minIntPtr(a *int, b *int) *int {
	if a == nil {
		return b
	}
	if b == nil || *a <= *b {
		return a
	}
	return b
}

func minDecimalPtr(a *decimal.Decimal, b *decimal.Decimal) *decimal.Decimal {
	if a == nil {
		return b
	}
	if b == nil || a.Cmp(*b) <= 0 {
		return a
	}
	return b
}

func mergeFeeType(a string, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" {
		return b
	}
	if b == "" || a == b {
		return a
	}
	if a == types.FeeTypeHybrid || b == types.FeeTypeHybrid {
		return types.FeeTypeHybrid
	}
	return types.FeeTypeHybrid
}

func loadPreviousRails(ctx context.Context, q dbtx, exchangeID int64) ([]diff.Rail, error) {
	rows, err := q.Query(ctx, `
		SELECT id, exchange_id, coin_id, chain_id,
		       deposit_enabled, withdraw_enabled, deposit_confirmations,
		       withdraw_min::text, withdraw_fee::text, withdraw_fee_type,
		       withdraw_fee_percent::text,
		       contract_address,
		       deposit_off_started_at, withdraw_off_started_at,
		       is_active, missing_since, missing_count, is_initial, last_seen_at
		FROM rails
		WHERE exchange_id = $1
	`, exchangeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rails []diff.Rail
	for rows.Next() {
		var rail diff.Rail
		var depositConfirmations pgtype.Int4
		var withdrawMin pgtype.Text
		var withdrawFee pgtype.Text
		var withdrawFeeType pgtype.Text
		var withdrawFeePercent pgtype.Text
		var contractAddress pgtype.Text
		var depositOffStartedAt pgtype.Timestamptz
		var withdrawOffStartedAt pgtype.Timestamptz
		var missingSince pgtype.Timestamptz
		if err := rows.Scan(
			&rail.ID,
			&rail.Key.ExchangeID,
			&rail.Key.CoinID,
			&rail.Key.ChainID,
			&rail.DepositEnabled,
			&rail.WithdrawEnabled,
			&depositConfirmations,
			&withdrawMin,
			&withdrawFee,
			&withdrawFeeType,
			&withdrawFeePercent,
			&contractAddress,
			&depositOffStartedAt,
			&withdrawOffStartedAt,
			&rail.IsActive,
			&missingSince,
			&rail.MissingCount,
			&rail.IsInitial,
			&rail.LastSeenAt,
		); err != nil {
			return nil, err
		}
		rail.DepositConfirmations = intPtrFromInt4(depositConfirmations)
		rail.WithdrawMin = decimalPtrFromText(withdrawMin)
		rail.WithdrawFee = decimalPtrFromText(withdrawFee)
		if withdrawFeeType.Valid {
			rail.WithdrawFeeType = withdrawFeeType.String
		}
		rail.WithdrawFeePercent = decimalPtrFromText(withdrawFeePercent)
		if contractAddress.Valid {
			rail.ContractAddress = contractAddress.String
		}
		rail.DepositOffStartedAt = timePtrFromTimestamptz(depositOffStartedAt)
		rail.WithdrawOffStartedAt = timePtrFromTimestamptz(withdrawOffStartedAt)
		rail.MissingSince = timePtrFromTimestamptz(missingSince)
		rails = append(rails, rail)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rails, nil
}

type sqlWriter struct {
	tx pgx.Tx
}

func (w sqlWriter) UpsertRails(ctx context.Context, mutations []diff.RailMutation) error {
	for _, mutation := range mutations {
		rail := mutation.After
		if _, err := w.tx.Exec(ctx, `
			INSERT INTO rails (
				exchange_id, coin_id, chain_id,
				deposit_enabled, withdraw_enabled, deposit_confirmations,
				withdraw_min, withdraw_fee, withdraw_fee_type, withdraw_fee_percent,
				contract_address,
				deposit_off_started_at, withdraw_off_started_at,
				is_active, missing_since, missing_count, is_initial, last_seen_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7::numeric, $8::numeric, $9, $10::numeric, $11, $12, $13, $14, $15, $16, $17, $18)
			ON CONFLICT (exchange_id, coin_id, chain_id) DO UPDATE
			SET deposit_enabled = EXCLUDED.deposit_enabled,
			    withdraw_enabled = EXCLUDED.withdraw_enabled,
			    deposit_confirmations = EXCLUDED.deposit_confirmations,
			    withdraw_min = EXCLUDED.withdraw_min,
			    withdraw_fee = EXCLUDED.withdraw_fee,
			    withdraw_fee_type = EXCLUDED.withdraw_fee_type,
			    withdraw_fee_percent = EXCLUDED.withdraw_fee_percent,
			    contract_address = EXCLUDED.contract_address,
			    deposit_off_started_at = EXCLUDED.deposit_off_started_at,
			    withdraw_off_started_at = EXCLUDED.withdraw_off_started_at,
			    is_active = EXCLUDED.is_active,
			    missing_since = EXCLUDED.missing_since,
			    missing_count = EXCLUDED.missing_count,
			    is_initial = EXCLUDED.is_initial,
			    last_seen_at = EXCLUDED.last_seen_at
		`,
			rail.Key.ExchangeID,
			rail.Key.CoinID,
			rail.Key.ChainID,
			rail.DepositEnabled,
			rail.WithdrawEnabled,
			intArg(rail.DepositConfirmations),
			decimalArg(rail.WithdrawMin),
			decimalArg(rail.WithdrawFee),
			feeTypeArg(rail.WithdrawFeeType),
			decimalArg(rail.WithdrawFeePercent),
			textArg(rail.ContractAddress),
			timeArg(rail.DepositOffStartedAt),
			timeArg(rail.WithdrawOffStartedAt),
			rail.IsActive,
			timeArg(rail.MissingSince),
			rail.MissingCount,
			rail.IsInitial,
			rail.LastSeenAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func (w sqlWriter) InsertEvents(ctx context.Context, events []diff.Event) error {
	for _, event := range events {
		before, err := json.Marshal(event.Before)
		if err != nil {
			return fmt.Errorf("marshal before state: %w", err)
		}
		after, err := json.Marshal(event.After)
		if err != nil {
			return fmt.Errorf("marshal after state: %w", err)
		}
		railID := event.RailID
		if railID == 0 {
			return fmt.Errorf("event %s missing rail id", event.Type)
		}
		if _, err := w.tx.Exec(ctx, `
			INSERT INTO rail_events (
				rail_id, exchange_id, coin_id, chain_id,
				event_type, before, after, occurred_at
			)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8)
		`, railID, event.Key.ExchangeID, event.Key.CoinID, event.Key.ChainID, string(event.Type), before, after, event.OccurredAt); err != nil {
			return err
		}
	}
	return nil
}

func intPtrFromInt4(value pgtype.Int4) *int {
	if !value.Valid {
		return nil
	}
	converted := int(value.Int32)
	return &converted
}

func decimalPtrFromText(value pgtype.Text) *decimal.Decimal {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := decimal.NewFromString(value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func intArg(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func decimalArg(value *decimal.Decimal) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func feeTypeArg(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func textArg(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func timeArg(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

var _ diff.Writer = (*sqlWriter)(nil)
var _ dbtx = pgx.Tx(nil)
