package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/silhuzz/cexyrouter/internal/db"
	"github.com/silhuzz/cexyrouter/internal/envfile"
)

type targetRail struct {
	ID                  int64
	ExchangeSlug        string
	CoinSlug            string
	CoinSymbol          string
	FromChainSlug       string
	ToChainSlug         string
	OriginalWithdraw    bool
	OriginalOffStarted  pgtype.Timestamptz
	OriginalRouteCount  int
	EquivalentAssetMode bool
}

func chainLookupCondition(alias string, param string) string {
	return fmt.Sprintf(`(
		LOWER(%[1]s.slug) = LOWER(%[2]s)
		OR LOWER(%[1]s.symbol) = LOWER(%[2]s)
		OR LOWER(%[1]s.name) = LOWER(%[2]s)
		OR EXISTS (
			SELECT 1
			FROM chain_aliases route_chain_alias
			WHERE route_chain_alias.chain_id = %[1]s.id
			  AND (
				  LOWER(route_chain_alias.raw_symbol) = LOWER(%[2]s)
				  OR LOWER(route_chain_alias.raw_name) = LOWER(%[2]s)
				  OR LOWER(route_chain_alias.raw_network_id) = LOWER(%[2]s)
			  )
		)
	)`, alias, param)
}

func main() {
	var envPath string
	var coin string
	var fromChain string
	var toChain string
	var duration time.Duration
	var restore bool
	var equivalentAssets bool
	flag.StringVar(&envPath, "env", ".env", "env file to load")
	flag.StringVar(&coin, "coin", "usdt", "coin slug to outage")
	flag.StringVar(&fromChain, "from-chain", "ethereum", "source/deposit chain slug")
	flag.StringVar(&fromChain, "from", "ethereum", "alias for -from-chain")
	flag.StringVar(&toChain, "to-chain", "arbitrum", "destination/withdraw chain slug")
	flag.StringVar(&toChain, "to", "arbitrum", "alias for -to-chain")
	flag.DurationVar(&duration, "duration", 8*time.Second, "how long to keep the route down before restoring")
	flag.BoolVar(&restore, "restore", true, "restore the rail and emit withdraw_on after duration")
	flag.BoolVar(&equivalentAssets, "equivalent-assets", false, "allow destination asset-family equivalents")
	flag.Parse()

	if err := envfile.Load(envPath, false); err != nil {
		exit("load env", err)
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		exit("config", fmt.Errorf("DATABASE_URL is required"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), duration+30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		exit("connect database", err)
	}
	defer pool.Close()

	target, err := findTargetRail(ctx, pool, coin, fromChain, toChain, equivalentAssets)
	if err != nil {
		exit("find demo route", err)
	}
	if target.OriginalRouteCount == 0 {
		exit("find demo route", fmt.Errorf("no currently open route for %s %s -> %s", coin, fromChain, toChain))
	}

	eventID, err := closeWithdrawRail(ctx, pool, target)
	if err != nil {
		exit("close route", err)
	}
	afterClose, err := routeCount(ctx, pool, coin, fromChain, toChain, equivalentAssets)
	if err != nil {
		exit("count closed route", err)
	}

	fmt.Printf("demo outage down: event_id=%d exchange=%s coin=%s from=%s to=%s routes_before=%d routes_after=%d\n", eventID, target.ExchangeSlug, target.CoinSymbol, target.FromChainSlug, target.ToChainSlug, target.OriginalRouteCount, afterClose)

	if !restore {
		fmt.Println("restore=false; leaving route down")
		return
	}

	if duration > 0 {
		time.Sleep(duration)
	}

	restoreEventID, err := restoreWithdrawRail(ctx, pool, target)
	if err != nil {
		exit("restore route", err)
	}
	afterRestore, err := routeCount(ctx, pool, coin, fromChain, toChain, equivalentAssets)
	if err != nil {
		exit("count restored route", err)
	}
	fmt.Printf("demo outage restored: event_id=%d routes_after_restore=%d\n", restoreEventID, afterRestore)
}

func findTargetRail(ctx context.Context, pool *pgxpool.Pool, coin string, fromChain string, toChain string, equivalentAssets bool) (targetRail, error) {
	count, err := routeCount(ctx, pool, coin, fromChain, toChain, equivalentAssets)
	if err != nil {
		return targetRail{}, err
	}

	coinJoinCondition := "w.exchange_id = d.exchange_id AND w.coin_id = d.coin_id"
	if equivalentAssets {
		coinJoinCondition = `
			w.exchange_id = d.exchange_id
			AND (
				w.coin_id = d.coin_id
				OR EXISTS (
					SELECT 1
					FROM asset_family_members deposit_member
					JOIN asset_family_members withdraw_member
					  ON withdraw_member.family_id = deposit_member.family_id
					WHERE deposit_member.coin_id = d.coin_id
					  AND withdraw_member.coin_id = w.coin_id
				)
			)
		`
	}

	sql := fmt.Sprintf(`
		SELECT w.id,
		       e.slug,
		       deposit_coin.slug,
		       withdraw_coin.symbol,
		       from_chain.slug,
		       to_chain.slug,
		       w.withdraw_enabled,
		       w.withdraw_off_started_at
		FROM rails d
		JOIN rails w ON %s
		JOIN exchanges e ON e.id = w.exchange_id
		JOIN coins deposit_coin ON deposit_coin.id = d.coin_id
		JOIN coins withdraw_coin ON withdraw_coin.id = w.coin_id
		JOIN chains from_chain ON from_chain.id = d.chain_id
		JOIN chains to_chain ON to_chain.id = w.chain_id
		WHERE deposit_coin.slug = $1
		  AND %s
		  AND %s
		  AND d.is_active = TRUE
		  AND w.is_active = TRUE
		  AND d.deposit_enabled = TRUE
		  AND w.withdraw_enabled = TRUE
		ORDER BY e.slug
		LIMIT 1
	`, coinJoinCondition, chainLookupCondition("from_chain", "$2"), chainLookupCondition("to_chain", "$3"))

	target := targetRail{
		OriginalRouteCount:  count,
		EquivalentAssetMode: equivalentAssets,
	}
	err = pool.QueryRow(ctx, sql, clean(coin), clean(fromChain), clean(toChain)).Scan(
		&target.ID,
		&target.ExchangeSlug,
		&target.CoinSlug,
		&target.CoinSymbol,
		&target.FromChainSlug,
		&target.ToChainSlug,
		&target.OriginalWithdraw,
		&target.OriginalOffStarted,
	)
	return target, err
}

func closeWithdrawRail(ctx context.Context, pool *pgxpool.Pool, target targetRail) (int64, error) {
	var eventID int64
	err := pool.QueryRow(ctx, `
		WITH target AS (
			SELECT id, exchange_id, coin_id, chain_id, withdraw_enabled AS old_withdraw
			FROM rails
			WHERE id = $1
		), updated AS (
			UPDATE rails r
			SET withdraw_enabled = FALSE,
			    withdraw_off_started_at = now(),
			    last_seen_at = now()
			FROM target
			WHERE r.id = target.id
			RETURNING r.id, r.exchange_id, r.coin_id, r.chain_id, r.withdraw_enabled, target.old_withdraw
		)
		INSERT INTO rail_events (rail_id, exchange_id, coin_id, chain_id, event_type, before, after)
		SELECT id,
		       exchange_id,
		       coin_id,
		       chain_id,
		       'withdraw_off',
		       jsonb_build_object('withdraw_enabled', old_withdraw),
		       jsonb_build_object('withdraw_enabled', withdraw_enabled)
		FROM updated
		RETURNING id
	`, target.ID).Scan(&eventID)
	return eventID, err
}

func restoreWithdrawRail(ctx context.Context, pool *pgxpool.Pool, target targetRail) (int64, error) {
	var offStartedAt any
	if target.OriginalOffStarted.Valid {
		offStartedAt = target.OriginalOffStarted.Time
	}

	var eventID int64
	err := pool.QueryRow(ctx, `
		WITH target AS (
			SELECT id, exchange_id, coin_id, chain_id, withdraw_enabled AS old_withdraw
			FROM rails
			WHERE id = $1
		), updated AS (
			UPDATE rails r
			SET withdraw_enabled = $2,
			    withdraw_off_started_at = $3,
			    last_seen_at = now()
			FROM target
			WHERE r.id = target.id
			RETURNING r.id, r.exchange_id, r.coin_id, r.chain_id, r.withdraw_enabled, target.old_withdraw
		)
		INSERT INTO rail_events (rail_id, exchange_id, coin_id, chain_id, event_type, before, after)
		SELECT id,
		       exchange_id,
		       coin_id,
		       chain_id,
		       CASE WHEN withdraw_enabled THEN 'withdraw_on' ELSE 'withdraw_off' END,
		       jsonb_build_object('withdraw_enabled', old_withdraw),
		       jsonb_build_object('withdraw_enabled', withdraw_enabled)
		FROM updated
		RETURNING id
	`, target.ID, target.OriginalWithdraw, offStartedAt).Scan(&eventID)
	return eventID, err
}

func routeCount(ctx context.Context, pool *pgxpool.Pool, coin string, fromChain string, toChain string, equivalentAssets bool) (int, error) {
	coinJoinCondition := "w.exchange_id = d.exchange_id AND w.coin_id = d.coin_id"
	if equivalentAssets {
		coinJoinCondition = `
			w.exchange_id = d.exchange_id
			AND (
				w.coin_id = d.coin_id
				OR EXISTS (
					SELECT 1
					FROM asset_family_members deposit_member
					JOIN asset_family_members withdraw_member
					  ON withdraw_member.family_id = deposit_member.family_id
					WHERE deposit_member.coin_id = d.coin_id
					  AND withdraw_member.coin_id = w.coin_id
				)
			)
		`
	}

	sql := fmt.Sprintf(`
		SELECT count(*)
		FROM rails d
		JOIN rails w ON %s
		JOIN coins deposit_coin ON deposit_coin.id = d.coin_id
		JOIN chains from_chain ON from_chain.id = d.chain_id
		JOIN chains to_chain ON to_chain.id = w.chain_id
		WHERE deposit_coin.slug = $1
		  AND %s
		  AND %s
		  AND d.is_active = TRUE
		  AND w.is_active = TRUE
		  AND d.deposit_enabled = TRUE
		  AND w.withdraw_enabled = TRUE
	`, coinJoinCondition, chainLookupCondition("from_chain", "$2"), chainLookupCondition("to_chain", "$3"))

	var count int
	err := pool.QueryRow(ctx, sql, clean(coin), clean(fromChain), clean(toChain)).Scan(&count)
	return count, err
}

func clean(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func exit(label string, err error) {
	slog.Error(label, "error", err)
	os.Exit(1)
}
