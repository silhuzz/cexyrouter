package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/silhuzz/cexyrouter/internal/chains"
	"github.com/silhuzz/cexyrouter/internal/db"
	"github.com/silhuzz/cexyrouter/internal/envfile"
)

type chainRow struct {
	ID        int64
	Slug      string
	Symbol    string
	Name      string
	Rails     int64
	Coins     int64
	Exchanges int64
}

type chainCandidate struct {
	Source    chainRow
	Target    chainRow
	Rails     int64
	Conflicts int64
	Aliases   []string
}

type symbolGroup struct {
	Symbol string
	Coins  []coinSummary
	Rails  int64
}

type coinSummary struct {
	Slug      string
	Symbol    string
	Name      string
	Rails     int64
	Exchanges int64
}

func main() {
	envPath := flag.String("env", ".env", "env file to load")
	top := flag.Int("top", 30, "maximum rows per section")
	strict := flag.Bool("strict", false, "exit non-zero when merge candidates remain")
	flag.Parse()

	if err := envfile.Load(*envPath, false); err != nil {
		exit("load env", err)
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		exit("config", fmt.Errorf("DATABASE_URL is required"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		exit("connect database", err)
	}
	defer pool.Close()

	chainCandidates, err := loadChainCandidates(ctx, pool)
	if err != nil {
		exit("load chain candidates", err)
	}
	unresolved, err := loadUnresolvedVariantChains(ctx, pool, *top)
	if err != nil {
		exit("load unresolved chains", err)
	}
	coinGroups, err := loadCoinSymbolGroups(ctx, pool, *top)
	if err != nil {
		exit("load coin groups", err)
	}

	printChainCandidates(chainCandidates, *top)
	printUnresolvedChains(unresolved)
	printCoinGroups(coinGroups)

	fmt.Printf("alias audit summary: chain_merge_candidates=%d unresolved_variant_chains=%d duplicate_coin_symbols=%d\n", len(chainCandidates), len(unresolved), len(coinGroups))
	if *strict && len(chainCandidates) > 0 {
		os.Exit(1)
	}
}

func loadChainCandidates(ctx context.Context, q queryer) ([]chainCandidate, error) {
	chainsBySlug, err := loadChains(ctx, q)
	if err != nil {
		return nil, err
	}

	candidates := make([]chainCandidate, 0)
	for _, source := range chainsBySlug {
		targetSlug, ok := chains.CanonicalSlug(source.Symbol, source.Name)
		if !ok || targetSlug == source.Slug {
			continue
		}
		target, ok := chainsBySlug[targetSlug]
		if !ok {
			continue
		}
		stats, err := loadCandidateStats(ctx, q, source.ID, target.ID)
		if err != nil {
			return nil, err
		}
		aliases, err := loadAliasExamples(ctx, q, source.ID, 4)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, chainCandidate{
			Source:    source,
			Target:    target,
			Rails:     stats.rails,
			Conflicts: stats.conflicts,
			Aliases:   aliases,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Rails != candidates[j].Rails {
			return candidates[i].Rails > candidates[j].Rails
		}
		return candidates[i].Source.Slug < candidates[j].Source.Slug
	})
	return candidates, nil
}

func loadChains(ctx context.Context, q queryer) (map[string]chainRow, error) {
	rows, err := q.Query(ctx, `
		SELECT ch.id,
		       ch.slug,
		       ch.symbol,
		       ch.name,
		       count(r.id) AS rails,
		       count(DISTINCT r.coin_id) AS coins,
		       count(DISTINCT r.exchange_id) AS exchanges
		FROM chains ch
		LEFT JOIN rails r ON r.chain_id = ch.id AND r.is_active = TRUE
		GROUP BY ch.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]chainRow)
	for rows.Next() {
		var row chainRow
		if err := rows.Scan(&row.ID, &row.Slug, &row.Symbol, &row.Name, &row.Rails, &row.Coins, &row.Exchanges); err != nil {
			return nil, err
		}
		out[row.Slug] = row
	}
	return out, rows.Err()
}

type candidateStats struct {
	rails     int64
	conflicts int64
}

func loadCandidateStats(ctx context.Context, q queryer, sourceID int64, targetID int64) (candidateStats, error) {
	var stats candidateStats
	err := q.QueryRow(ctx, `
		SELECT count(src.id),
		       count(existing.id)
		FROM rails src
		LEFT JOIN rails existing
		  ON existing.exchange_id = src.exchange_id
		 AND existing.coin_id = src.coin_id
		 AND existing.chain_id = $2
		WHERE src.chain_id = $1
		  AND src.is_active = TRUE
	`, sourceID, targetID).Scan(&stats.rails, &stats.conflicts)
	return stats, err
}

func loadAliasExamples(ctx context.Context, q queryer, chainID int64, limit int) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT e.slug, ca.raw_symbol, ca.raw_name, ca.raw_network_id
		FROM chain_aliases ca
		JOIN exchanges e ON e.id = ca.exchange_id
		WHERE ca.chain_id = $1
		ORDER BY e.slug, ca.raw_symbol, ca.raw_name, ca.raw_network_id
		LIMIT $2
	`, chainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	aliases := make([]string, 0, limit)
	for rows.Next() {
		var exchange, symbol, name, networkID string
		if err := rows.Scan(&exchange, &symbol, &name, &networkID); err != nil {
			return nil, err
		}
		aliases = append(aliases, fmt.Sprintf("%s:%s/%s/%s", exchange, symbol, name, networkID))
	}
	return aliases, rows.Err()
}

func loadUnresolvedVariantChains(ctx context.Context, q queryer, limit int) ([]chainRow, error) {
	rows, err := q.Query(ctx, `
		SELECT ch.id,
		       ch.slug,
		       ch.symbol,
		       ch.name,
		       count(r.id) AS rails,
		       count(DISTINCT r.coin_id) AS coins,
		       count(DISTINCT r.exchange_id) AS exchanges
		FROM chains ch
		JOIN rails r ON r.chain_id = ch.id AND r.is_active = TRUE
		WHERE (
			ch.slug LIKE '%evm'
			OR ch.slug LIKE '%eth'
			OR ch.slug LIKE '%-chain'
			OR ch.slug LIKE '%-pos'
			OR ch.slug LIKE 'zk%era'
			OR ch.slug LIKE '%-era'
		)
		GROUP BY ch.id
		ORDER BY rails DESC, ch.slug
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []chainRow
	for rows.Next() {
		var row chainRow
		if err := rows.Scan(&row.ID, &row.Slug, &row.Symbol, &row.Name, &row.Rails, &row.Coins, &row.Exchanges); err != nil {
			return nil, err
		}
		if canonical, ok := chains.CanonicalSlug(row.Symbol, row.Name); ok && canonical != row.Slug {
			continue
		}
		if isKnownCanonicalChain(row.Slug) {
			continue
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func isKnownCanonicalChain(slug string) bool {
	for _, canonical := range chains.Synonyms {
		if canonical == slug {
			return true
		}
	}
	return false
}

func loadCoinSymbolGroups(ctx context.Context, q queryer, limit int) ([]symbolGroup, error) {
	rows, err := q.Query(ctx, `
		WITH active_coin_stats AS (
			SELECT c.id,
			       c.slug,
			       c.symbol,
			       c.name,
			       count(r.id) AS rails,
			       count(DISTINCT r.exchange_id) AS exchanges
			FROM coins c
			JOIN rails r ON r.coin_id = c.id AND r.is_active = TRUE
			GROUP BY c.id
		), duplicate_symbols AS (
			SELECT upper(symbol) AS symbol_key,
			       sum(rails) AS rails
			FROM active_coin_stats
			GROUP BY upper(symbol)
			HAVING count(*) > 1
			ORDER BY rails DESC, symbol_key
			LIMIT $1
		)
		SELECT d.symbol_key, s.slug, s.symbol, s.name, s.rails, s.exchanges
		FROM duplicate_symbols d
		JOIN active_coin_stats s ON upper(s.symbol) = d.symbol_key
		ORDER BY d.rails DESC, d.symbol_key, s.rails DESC, s.slug
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bySymbol := make(map[string]*symbolGroup)
	order := make([]string, 0)
	for rows.Next() {
		var symbol string
		var coin coinSummary
		if err := rows.Scan(&symbol, &coin.Slug, &coin.Symbol, &coin.Name, &coin.Rails, &coin.Exchanges); err != nil {
			return nil, err
		}
		group := bySymbol[symbol]
		if group == nil {
			group = &symbolGroup{Symbol: symbol}
			bySymbol[symbol] = group
			order = append(order, symbol)
		}
		group.Coins = append(group.Coins, coin)
		group.Rails += coin.Rails
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	groups := make([]symbolGroup, 0, len(order))
	for _, symbol := range order {
		groups = append(groups, *bySymbol[symbol])
	}
	return groups, nil
}

func printChainCandidates(candidates []chainCandidate, top int) {
	fmt.Println("chain merge candidates:")
	if len(candidates) == 0 {
		fmt.Println("  none")
		return
	}
	if top > 0 && len(candidates) > top {
		candidates = candidates[:top]
	}
	for _, candidate := range candidates {
		fmt.Printf("  %s -> %s rails=%d conflicts=%d coins=%d exchanges=%d aliases=%s\n",
			candidate.Source.Slug,
			candidate.Target.Slug,
			candidate.Rails,
			candidate.Conflicts,
			candidate.Source.Coins,
			candidate.Source.Exchanges,
			strings.Join(candidate.Aliases, ", "),
		)
	}
}

func printUnresolvedChains(rows []chainRow) {
	fmt.Println("unresolved variant-looking chains:")
	if len(rows) == 0 {
		fmt.Println("  none")
		return
	}
	for _, row := range rows {
		fmt.Printf("  %s symbol=%s name=%s rails=%d coins=%d exchanges=%d\n", row.Slug, row.Symbol, row.Name, row.Rails, row.Coins, row.Exchanges)
	}
}

func printCoinGroups(groups []symbolGroup) {
	fmt.Println("duplicate active coin symbols:")
	if len(groups) == 0 {
		fmt.Println("  none")
		return
	}
	for _, group := range groups {
		parts := make([]string, 0, len(group.Coins))
		for _, coin := range group.Coins {
			parts = append(parts, fmt.Sprintf("%s(%d rails)", coin.Slug, coin.Rails))
		}
		fmt.Printf("  %s rails=%d coins=%s\n", group.Symbol, group.Rails, strings.Join(parts, ", "))
	}
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func exit(label string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
	os.Exit(1)
}
