package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/silhuzz/cexyrouter/internal/db"
	"github.com/silhuzz/cexyrouter/internal/envfile"
)

type exchangeCoverage struct {
	Slug          string
	Name          string
	ActiveRails   int64
	Coins         int64
	Chains        int64
	DepositsOpen  int64
	WithdrawsOpen int64
}

type routeSummary struct {
	Candidates       int64
	Pairs            int64
	Coins            int64
	Exchanges        int64
	UnknownFeeRoutes int64
}

type routeFinding struct {
	Exchange         string
	Coin             string
	CoinSlug         string
	FromChain        string
	ToChain          string
	Routes           int64
	UnknownFeeRoutes int64
}

type railGap struct {
	Exchange string
	Coin     string
	CoinSlug string
	Chain    string
	Reason   string
}

const adapterPollHistorySQL = `(af.last_successful_poll IS NOT NULL OR af.last_attempt IS NOT NULL OR af.last_error IS NOT NULL)`

func main() {
	envPath := flag.String("env", ".env", "env file to load")
	top := flag.Int("top", 25, "maximum rows per detailed section")
	strict := flag.Bool("strict", false, "exit non-zero if the audit has hard failures")
	requireKnownFees := flag.Bool("require-known-fees", false, "treat route candidates with unknown fee metadata as failures")
	equivalentAssets := flag.Bool("equivalent-assets", false, "include asset-family equivalent destination coins")
	flag.Parse()

	if err := envfile.Load(*envPath, false); err != nil {
		exit("load env", err)
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		exit("config", fmt.Errorf("DATABASE_URL is required"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		exit("connect database", err)
	}
	defer pool.Close()

	coverage, err := loadExchangeCoverage(ctx, pool)
	if err != nil {
		exit("load exchange coverage", err)
	}
	summary, err := loadRouteSummary(ctx, pool, *equivalentAssets)
	if err != nil {
		exit("load route summary", err)
	}
	unknownFees, err := loadUnknownFeeRoutes(ctx, pool, *equivalentAssets, *top)
	if err != nil {
		exit("load unknown fee routes", err)
	}
	depositOnly, err := loadRailGaps(ctx, pool, "deposit_only", *top)
	if err != nil {
		exit("load deposit-only rails", err)
	}
	withdrawOnly, err := loadRailGaps(ctx, pool, "withdraw_only", *top)
	if err != nil {
		exit("load withdraw-only rails", err)
	}
	singleExchangePairs, err := loadSingleExchangePairs(ctx, pool, *equivalentAssets, *top)
	if err != nil {
		exit("load single-exchange route pairs", err)
	}

	mode := "same_asset"
	if *equivalentAssets {
		mode = "equivalent_asset_enabled"
	}
	fmt.Printf("route audit mode=%s\n", mode)
	printCoverage(coverage)
	printSummary(summary)
	printRouteFindings("routes with unknown fee metadata (served as fee n/a)", unknownFees)
	printRailGaps("open deposit rails with no open withdraw rail on same exchange/token", depositOnly)
	printRailGaps("open withdraw rails with no open deposit rail on same exchange/token", withdrawOnly)
	printRouteFindings("single-exchange route pairs worth eyeballing", singleExchangePairs)

	hardFailures := 0
	if len(coverage) == 0 {
		fmt.Println("hard failure: no integrated exchanges found")
		hardFailures++
	}
	if summary.Candidates == 0 {
		fmt.Println("hard failure: no open route candidates found")
		hardFailures++
	}
	if *requireKnownFees && summary.UnknownFeeRoutes > 0 {
		fmt.Printf("hard failure: %d route candidates have unknown fee metadata\n", summary.UnknownFeeRoutes)
		hardFailures++
	}

	fmt.Printf("route audit summary: exchanges=%d coins_with_routes=%d route_candidates=%d route_pairs=%d unknown_fee_routes=%d deposit_only_examples=%d withdraw_only_examples=%d single_exchange_pair_examples=%d hard_failures=%d\n",
		len(coverage),
		summary.Coins,
		summary.Candidates,
		summary.Pairs,
		summary.UnknownFeeRoutes,
		len(depositOnly),
		len(withdrawOnly),
		len(singleExchangePairs),
		hardFailures,
	)
	if *strict && hardFailures > 0 {
		os.Exit(1)
	}
}

func loadExchangeCoverage(ctx context.Context, q queryer) ([]exchangeCoverage, error) {
	rows, err := q.Query(ctx, `
		SELECT e.slug,
		       e.name,
		       count(r.id) FILTER (WHERE r.is_active = TRUE) AS active_rails,
		       count(DISTINCT r.coin_id) FILTER (WHERE r.is_active = TRUE) AS coins,
		       count(DISTINCT r.chain_id) FILTER (WHERE r.is_active = TRUE) AS chains,
		       count(r.id) FILTER (WHERE r.is_active = TRUE AND r.deposit_enabled = TRUE) AS deposits_open,
		       count(r.id) FILTER (WHERE r.is_active = TRUE AND r.withdraw_enabled = TRUE) AS withdraws_open
		FROM exchanges e
		JOIN adapter_freshness af ON af.exchange_id = e.id
		LEFT JOIN rails r ON r.exchange_id = e.id
		WHERE `+adapterPollHistorySQL+`
		GROUP BY e.id
		ORDER BY e.slug
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]exchangeCoverage, 0)
	for rows.Next() {
		var row exchangeCoverage
		if err := rows.Scan(&row.Slug, &row.Name, &row.ActiveRails, &row.Coins, &row.Chains, &row.DepositsOpen, &row.WithdrawsOpen); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadRouteSummary(ctx context.Context, q queryer, equivalentAssets bool) (routeSummary, error) {
	var summary routeSummary
	err := q.QueryRow(ctx, fmt.Sprintf(`
		WITH candidates AS (
			SELECT d.exchange_id,
			       d.coin_id,
			       d.chain_id AS from_chain_id,
			       w.coin_id AS to_coin_id,
			       w.chain_id AS to_chain_id,
			       `+withdrawFeeKnownSQL("w")+` AS fee_known
			FROM rails d
			JOIN rails w ON %s
			JOIN exchanges e ON e.id = d.exchange_id
			JOIN adapter_freshness af ON af.exchange_id = e.id
			WHERE `+adapterPollHistorySQL+`
			  AND d.is_active = TRUE
			  AND w.is_active = TRUE
			  AND d.deposit_enabled = TRUE
			  AND w.withdraw_enabled = TRUE
		)
		SELECT count(*),
		       count(DISTINCT (coin_id, from_chain_id, to_coin_id, to_chain_id)),
		       count(DISTINCT coin_id),
		       count(DISTINCT exchange_id),
		       count(*) FILTER (WHERE fee_known = FALSE)
		FROM candidates
	`, routeCoinJoinCondition(equivalentAssets))).Scan(
		&summary.Candidates,
		&summary.Pairs,
		&summary.Coins,
		&summary.Exchanges,
		&summary.UnknownFeeRoutes,
	)
	return summary, err
}

func loadUnknownFeeRoutes(ctx context.Context, q queryer, equivalentAssets bool, limit int) ([]routeFinding, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT e.slug,
		       deposit_coin.symbol,
		       deposit_coin.slug,
		       from_chain.slug,
		       to_chain.slug,
		       count(*) AS routes,
		       count(*) FILTER (WHERE `+withdrawFeeKnownSQL("w")+` = FALSE) AS unknown_fee_routes
		FROM rails d
		JOIN rails w ON %s
		JOIN exchanges e ON e.id = d.exchange_id
		JOIN adapter_freshness af ON af.exchange_id = e.id
		JOIN coins deposit_coin ON deposit_coin.id = d.coin_id
		JOIN chains from_chain ON from_chain.id = d.chain_id
		JOIN chains to_chain ON to_chain.id = w.chain_id
		WHERE `+adapterPollHistorySQL+`
		  AND d.is_active = TRUE
		  AND w.is_active = TRUE
		  AND d.deposit_enabled = TRUE
		  AND w.withdraw_enabled = TRUE
		  AND `+withdrawFeeKnownSQL("w")+` = FALSE
		GROUP BY e.slug, deposit_coin.symbol, deposit_coin.slug, from_chain.slug, to_chain.slug
		ORDER BY routes DESC, e.slug, deposit_coin.slug, from_chain.slug, to_chain.slug
		LIMIT $1
	`, routeCoinJoinCondition(equivalentAssets)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRouteFindings(rows)
}

func loadSingleExchangePairs(ctx context.Context, q queryer, equivalentAssets bool, limit int) ([]routeFinding, error) {
	rows, err := q.Query(ctx, fmt.Sprintf(`
		WITH pair_support AS (
			SELECT deposit_coin.symbol,
			       deposit_coin.slug AS coin_slug,
			       from_chain.slug AS from_chain,
			       to_chain.slug AS to_chain,
			       count(*) AS routes,
			       count(*) FILTER (WHERE `+withdrawFeeKnownSQL("w")+` = FALSE) AS unknown_fee_routes,
			       count(DISTINCT e.slug) AS exchanges,
			       min(e.slug) AS exchange
			FROM rails d
			JOIN rails w ON %s
			JOIN exchanges e ON e.id = d.exchange_id
			JOIN adapter_freshness af ON af.exchange_id = e.id
			JOIN coins deposit_coin ON deposit_coin.id = d.coin_id
			JOIN chains from_chain ON from_chain.id = d.chain_id
			JOIN chains to_chain ON to_chain.id = w.chain_id
			WHERE `+adapterPollHistorySQL+`
			  AND d.is_active = TRUE
			  AND w.is_active = TRUE
			  AND d.deposit_enabled = TRUE
			  AND w.withdraw_enabled = TRUE
			GROUP BY deposit_coin.symbol, deposit_coin.slug, from_chain.slug, to_chain.slug
		)
		SELECT exchange,
		       symbol,
		       coin_slug,
		       from_chain,
		       to_chain,
		       routes,
		       unknown_fee_routes
		FROM pair_support
		WHERE exchanges = 1
		  AND from_chain <> to_chain
		ORDER BY unknown_fee_routes DESC, routes DESC, coin_slug, from_chain, to_chain
		LIMIT $1
	`, routeCoinJoinCondition(equivalentAssets)), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRouteFindings(rows)
}

func loadRailGaps(ctx context.Context, q queryer, mode string, limit int) ([]railGap, error) {
	var predicate string
	switch mode {
	case "deposit_only":
		predicate = `
			r.deposit_enabled = TRUE
			AND NOT EXISTS (
				SELECT 1
				FROM rails peer
				WHERE peer.exchange_id = r.exchange_id
				  AND peer.coin_id = r.coin_id
				  AND peer.is_active = TRUE
				  AND peer.withdraw_enabled = TRUE
			)
		`
	case "withdraw_only":
		predicate = `
			r.withdraw_enabled = TRUE
			AND NOT EXISTS (
				SELECT 1
				FROM rails peer
				WHERE peer.exchange_id = r.exchange_id
				  AND peer.coin_id = r.coin_id
				  AND peer.is_active = TRUE
				  AND peer.deposit_enabled = TRUE
			)
		`
	default:
		return nil, fmt.Errorf("unsupported rail gap mode %q", mode)
	}

	rows, err := q.Query(ctx, fmt.Sprintf(`
		SELECT e.slug,
		       c.symbol,
		       c.slug,
		       ch.slug,
		       $2::text AS reason
		FROM rails r
		JOIN exchanges e ON e.id = r.exchange_id
		JOIN adapter_freshness af ON af.exchange_id = e.id
		JOIN coins c ON c.id = r.coin_id
		JOIN chains ch ON ch.id = r.chain_id
		WHERE `+adapterPollHistorySQL+`
		  AND r.is_active = TRUE
		  AND %s
		ORDER BY e.slug, c.slug, ch.slug
		LIMIT $1
	`, predicate), limit, mode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	gaps := make([]railGap, 0)
	for rows.Next() {
		var gap railGap
		if err := rows.Scan(&gap.Exchange, &gap.Coin, &gap.CoinSlug, &gap.Chain, &gap.Reason); err != nil {
			return nil, err
		}
		gaps = append(gaps, gap)
	}
	return gaps, rows.Err()
}

func scanRouteFindings(rows pgx.Rows) ([]routeFinding, error) {
	findings := make([]routeFinding, 0)
	for rows.Next() {
		var row routeFinding
		if err := rows.Scan(
			&row.Exchange,
			&row.Coin,
			&row.CoinSlug,
			&row.FromChain,
			&row.ToChain,
			&row.Routes,
			&row.UnknownFeeRoutes,
		); err != nil {
			return nil, err
		}
		findings = append(findings, row)
	}
	return findings, rows.Err()
}

func printCoverage(rows []exchangeCoverage) {
	fmt.Println("integrated exchange coverage:")
	if len(rows) == 0 {
		fmt.Println("  none")
		return
	}
	for _, row := range rows {
		fmt.Printf("  %-9s rails=%5d coins=%4d chains=%4d deposits_open=%5d withdraws_open=%5d\n",
			row.Slug,
			row.ActiveRails,
			row.Coins,
			row.Chains,
			row.DepositsOpen,
			row.WithdrawsOpen,
		)
	}
}

func printSummary(summary routeSummary) {
	fmt.Println("route candidate coverage:")
	fmt.Printf("  candidates=%d pairs=%d coins=%d exchanges=%d unknown_fee_routes=%d\n",
		summary.Candidates,
		summary.Pairs,
		summary.Coins,
		summary.Exchanges,
		summary.UnknownFeeRoutes,
	)
}

func printRouteFindings(title string, rows []routeFinding) {
	fmt.Println(title + ":")
	if len(rows) == 0 {
		fmt.Println("  none")
		return
	}
	for _, row := range rows {
		fmt.Printf("  %-9s %-8s %-12s -> %-12s routes=%d unknown_fee=%d\n",
			row.Exchange,
			row.Coin,
			row.FromChain,
			row.ToChain,
			row.Routes,
			row.UnknownFeeRoutes,
		)
	}
}

func printRailGaps(title string, rows []railGap) {
	fmt.Println(title + ":")
	if len(rows) == 0 {
		fmt.Println("  none")
		return
	}
	for _, row := range rows {
		fmt.Printf("  %-9s %-8s %-12s reason=%s\n", row.Exchange, row.Coin, row.Chain, row.Reason)
	}
}

func routeCoinJoinCondition(equivalentAssets bool) string {
	if !equivalentAssets {
		return "w.exchange_id = d.exchange_id AND w.coin_id = d.coin_id"
	}
	return `
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

func withdrawFeeKnownSQL(alias string) string {
	return fmt.Sprintf(`(
		CASE
			WHEN %[1]s.withdraw_fee_type = 'percent' THEN %[1]s.withdraw_fee_percent IS NOT NULL
			WHEN %[1]s.withdraw_fee_type = 'hybrid' THEN %[1]s.withdraw_fee IS NOT NULL OR %[1]s.withdraw_fee_percent IS NOT NULL
			ELSE %[1]s.withdraw_fee IS NOT NULL
		END
	)`, alias)
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func exit(label string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
	os.Exit(1)
}
