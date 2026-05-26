package normalizer

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type SQLStore struct {
	q PgxQuerier
}

func NewSQLStore(q PgxQuerier) *SQLStore {
	return &SQLStore{q: q}
}

func (s *SQLStore) ExchangeID(ctx context.Context, exchangeSlug string) (int64, error) {
	var id int64
	if err := s.q.QueryRow(ctx, `
		SELECT id
		FROM exchanges
		WHERE slug = $1
	`, strings.ToLower(clean(exchangeSlug))).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *SQLStore) LoadCoinAliases(ctx context.Context, exchangeID int64) ([]CoinAliasRef, error) {
	rows, err := s.q.Query(ctx, `
		SELECT ca.exchange_id, ca.raw_symbol, ca.raw_name,
		       c.id, c.slug, c.symbol, c.name
		FROM coin_aliases ca
		JOIN coins c ON c.id = ca.coin_id
		WHERE ca.exchange_id = $1
	`, exchangeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []CoinAliasRef
	for rows.Next() {
		var alias CoinAliasRef
		if err := rows.Scan(
			&alias.ExchangeID,
			&alias.RawSymbol,
			&alias.RawName,
			&alias.Coin.ID,
			&alias.Coin.Slug,
			&alias.Coin.Symbol,
			&alias.Coin.Name,
		); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aliases, nil
}

func (s *SQLStore) FindCoinAlias(ctx context.Context, exchangeID int64, rawSymbol string, rawName string) (CoinRef, bool, error) {
	var coin CoinRef
	err := s.q.QueryRow(ctx, `
		SELECT c.id, c.slug, c.symbol, c.name
		FROM coin_aliases ca
		JOIN coins c ON c.id = ca.coin_id
		WHERE ca.exchange_id = $1
		  AND ca.raw_symbol = $2
		  AND ca.raw_name = $3
	`, exchangeID, clean(rawSymbol), clean(rawName)).Scan(&coin.ID, &coin.Slug, &coin.Symbol, &coin.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return CoinRef{}, false, nil
	}
	if err != nil {
		return CoinRef{}, false, err
	}
	return coin, true, nil
}

func (s *SQLStore) FindCoinByContract(ctx context.Context, chainID int64, contractAddress string) (CoinRef, bool, error) {
	var coin CoinRef
	err := s.q.QueryRow(ctx, `
		SELECT c.id, c.slug, c.symbol, c.name
		FROM asset_contracts ac
		JOIN coins c ON c.id = ac.coin_id
		WHERE ac.chain_id = $1
		  AND ac.contract_address_normalized = $2
	`, chainID, normalizeContractAddress(contractAddress)).Scan(&coin.ID, &coin.Slug, &coin.Symbol, &coin.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return CoinRef{}, false, nil
	}
	if err != nil {
		return CoinRef{}, false, err
	}
	return coin, true, nil
}

func (s *SQLStore) FindCoinCanonical(ctx context.Context, symbol string, name string) (CoinRef, bool, error) {
	var coin CoinRef
	err := s.q.QueryRow(ctx, `
		SELECT id, slug, symbol, name
		FROM coins
		WHERE upper(symbol) = upper($1)
		   OR ($2 <> '' AND lower(name) = lower($2))
		ORDER BY CASE WHEN upper(symbol) = upper($1) THEN 0 ELSE 1 END, id
		LIMIT 1
	`, clean(symbol), clean(name)).Scan(&coin.ID, &coin.Slug, &coin.Symbol, &coin.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return CoinRef{}, false, nil
	}
	if err != nil {
		return CoinRef{}, false, err
	}
	return coin, true, nil
}

func (s *SQLStore) InsertCoin(ctx context.Context, symbol string, name string) (CoinRef, error) {
	symbol = clean(symbol)
	name = clean(name)
	if symbol == "" {
		return CoinRef{}, ErrMissingCoinInput
	}
	if name == "" {
		name = symbol
	}

	var coin CoinRef
	if err := s.q.QueryRow(ctx, `
		INSERT INTO coins (slug, symbol, name, external_ids)
		VALUES ($1, $2, $3, '{}'::jsonb)
		ON CONFLICT (slug) DO UPDATE
		SET symbol = EXCLUDED.symbol,
		    name = CASE WHEN coins.name = '' THEN EXCLUDED.name ELSE coins.name END
		RETURNING id, slug, symbol, name
	`, slugify(symbol), symbol, name).Scan(&coin.ID, &coin.Slug, &coin.Symbol, &coin.Name); err != nil {
		return CoinRef{}, err
	}
	return coin, nil
}

func (s *SQLStore) UpsertCoinAlias(ctx context.Context, upsert CoinAliasUpsert) error {
	return s.UpsertCoinAliases(ctx, []CoinAliasUpsert{upsert})
}

func (s *SQLStore) UpsertCoinAliases(ctx context.Context, upserts []CoinAliasUpsert) error {
	if len(upserts) == 0 {
		return nil
	}

	exchangeIDs := make([]int32, 0, len(upserts))
	rawSymbols := make([]string, 0, len(upserts))
	rawNames := make([]string, 0, len(upserts))
	coinIDs := make([]int32, 0, len(upserts))
	seenAt := make([]time.Time, 0, len(upserts))
	for i, upsert := range upserts {
		exchangeID, err := int32FromInt64(upsert.ExchangeID)
		if err != nil {
			return err
		}
		coinID, err := int32FromInt64(upsert.CoinID)
		if err != nil {
			return err
		}
		exchangeIDs = append(exchangeIDs, exchangeID)
		rawSymbols = append(rawSymbols, clean(upsert.RawSymbol))
		rawNames = append(rawNames, clean(upsert.RawName))
		coinIDs = append(coinIDs, coinID)
		seenAt = append(seenAt, upsert.SeenAt)
		if rawSymbols[i] == "" {
			return ErrMissingCoinInput
		}
	}

	_, err := s.q.Exec(ctx, `
		INSERT INTO coin_aliases (
			exchange_id, raw_symbol, raw_name, coin_id, confidence, first_seen, last_seen
		)
		SELECT input.exchange_id, input.raw_symbol, input.raw_name, input.coin_id, 1, input.seen_at, input.seen_at
		FROM unnest(
			$1::int[], $2::text[], $3::text[], $4::int[], $5::timestamptz[]
		) AS input(exchange_id, raw_symbol, raw_name, coin_id, seen_at)
		ON CONFLICT (exchange_id, raw_symbol, raw_name) DO UPDATE
		SET coin_id = EXCLUDED.coin_id,
		    confidence = LEAST(coin_aliases.confidence + 1, 3),
		    last_seen = EXCLUDED.last_seen
	`, exchangeIDs, rawSymbols, rawNames, coinIDs, seenAt)
	return err
}

func (s *SQLStore) LoadChainAliases(ctx context.Context, exchangeID int64) ([]ChainAliasRef, error) {
	rows, err := s.q.Query(ctx, `
		SELECT ca.exchange_id, ca.raw_symbol, ca.raw_name, ca.raw_network_id,
		       c.id, c.slug, c.symbol, c.name
		FROM chain_aliases ca
		JOIN chains c ON c.id = ca.chain_id
		WHERE ca.exchange_id = $1
	`, exchangeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aliases []ChainAliasRef
	for rows.Next() {
		var alias ChainAliasRef
		if err := rows.Scan(
			&alias.ExchangeID,
			&alias.RawSymbol,
			&alias.RawName,
			&alias.RawNetworkID,
			&alias.Chain.ID,
			&alias.Chain.Slug,
			&alias.Chain.Symbol,
			&alias.Chain.Name,
		); err != nil {
			return nil, err
		}
		aliases = append(aliases, alias)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return aliases, nil
}

func (s *SQLStore) FindChainAlias(ctx context.Context, exchangeID int64, rawSymbol string, rawName string, rawNetworkID string) (ChainRef, bool, error) {
	var chain ChainRef
	err := s.q.QueryRow(ctx, `
		SELECT c.id, c.slug, c.symbol, c.name
		FROM chain_aliases ca
		JOIN chains c ON c.id = ca.chain_id
		WHERE ca.exchange_id = $1
		  AND ca.raw_symbol = $2
		  AND ca.raw_name = $3
		  AND ca.raw_network_id = $4
	`, exchangeID, clean(rawSymbol), clean(rawName), clean(rawNetworkID)).Scan(&chain.ID, &chain.Slug, &chain.Symbol, &chain.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChainRef{}, false, nil
	}
	if err != nil {
		return ChainRef{}, false, err
	}
	return chain, true, nil
}

func (s *SQLStore) FindChainCanonical(ctx context.Context, symbol string, name string) (ChainRef, bool, error) {
	var chain ChainRef
	err := s.q.QueryRow(ctx, `
		SELECT id, slug, symbol, name
		FROM chains
		WHERE upper(symbol) = upper($1)
		   OR ($2 <> '' AND lower(name) = lower($2))
		ORDER BY CASE WHEN upper(symbol) = upper($1) THEN 0 ELSE 1 END, id
		LIMIT 1
	`, clean(symbol), clean(name)).Scan(&chain.ID, &chain.Slug, &chain.Symbol, &chain.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChainRef{}, false, nil
	}
	if err != nil {
		return ChainRef{}, false, err
	}
	return chain, true, nil
}

func (s *SQLStore) FindChainBySlug(ctx context.Context, slug string) (ChainRef, bool, error) {
	var chain ChainRef
	err := s.q.QueryRow(ctx, `
		SELECT id, slug, symbol, name
		FROM chains
		WHERE slug = $1
	`, strings.ToLower(clean(slug))).Scan(&chain.ID, &chain.Slug, &chain.Symbol, &chain.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return ChainRef{}, false, nil
	}
	if err != nil {
		return ChainRef{}, false, err
	}
	return chain, true, nil
}

func (s *SQLStore) InsertChain(ctx context.Context, symbol string, name string) (ChainRef, error) {
	symbol = clean(symbol)
	name = clean(name)
	if symbol == "" && name == "" {
		return ChainRef{}, ErrMissingChainInput
	}
	if symbol == "" {
		symbol = name
	}
	if name == "" {
		name = symbol
	}

	var chain ChainRef
	if err := s.q.QueryRow(ctx, `
		INSERT INTO chains (slug, symbol, name)
		VALUES ($1, $2, $3)
		ON CONFLICT (slug) DO UPDATE
		SET symbol = EXCLUDED.symbol,
		    name = CASE WHEN chains.name = '' THEN EXCLUDED.name ELSE chains.name END
		RETURNING id, slug, symbol, name
	`, slugify(symbol), symbol, name).Scan(&chain.ID, &chain.Slug, &chain.Symbol, &chain.Name); err != nil {
		return ChainRef{}, err
	}
	return chain, nil
}

func (s *SQLStore) UpsertChainAlias(ctx context.Context, upsert ChainAliasUpsert) error {
	return s.UpsertChainAliases(ctx, []ChainAliasUpsert{upsert})
}

func (s *SQLStore) UpsertChainAliases(ctx context.Context, upserts []ChainAliasUpsert) error {
	if len(upserts) == 0 {
		return nil
	}

	exchangeIDs := make([]int32, 0, len(upserts))
	rawSymbols := make([]string, 0, len(upserts))
	rawNames := make([]string, 0, len(upserts))
	rawNetworkIDs := make([]string, 0, len(upserts))
	chainIDs := make([]int32, 0, len(upserts))
	seenAt := make([]time.Time, 0, len(upserts))
	for _, upsert := range upserts {
		exchangeID, err := int32FromInt64(upsert.ExchangeID)
		if err != nil {
			return err
		}
		chainID, err := int32FromInt64(upsert.ChainID)
		if err != nil {
			return err
		}
		rawSymbol := clean(upsert.RawSymbol)
		rawName := clean(upsert.RawName)
		if rawSymbol == "" && rawName == "" {
			return ErrMissingChainInput
		}
		exchangeIDs = append(exchangeIDs, exchangeID)
		rawSymbols = append(rawSymbols, rawSymbol)
		rawNames = append(rawNames, rawName)
		rawNetworkIDs = append(rawNetworkIDs, clean(upsert.RawNetworkID))
		chainIDs = append(chainIDs, chainID)
		seenAt = append(seenAt, upsert.SeenAt)
	}

	_, err := s.q.Exec(ctx, `
		INSERT INTO chain_aliases (
			exchange_id, raw_symbol, raw_name, raw_network_id, chain_id, confidence, first_seen, last_seen
		)
		SELECT input.exchange_id, input.raw_symbol, input.raw_name, input.raw_network_id, input.chain_id, 1, input.seen_at, input.seen_at
		FROM unnest(
			$1::int[], $2::text[], $3::text[], $4::text[], $5::int[], $6::timestamptz[]
		) AS input(exchange_id, raw_symbol, raw_name, raw_network_id, chain_id, seen_at)
		ON CONFLICT (exchange_id, raw_symbol, raw_name, raw_network_id) DO UPDATE
		SET chain_id = EXCLUDED.chain_id,
		    confidence = LEAST(chain_aliases.confidence + 1, 3),
		    last_seen = EXCLUDED.last_seen
	`, exchangeIDs, rawSymbols, rawNames, rawNetworkIDs, chainIDs, seenAt)
	return err
}

func int32FromInt64(value int64) (int32, error) {
	if value < -1<<31 || value > 1<<31-1 {
		return 0, errors.New("id is outside int4 range")
	}
	return int32(value), nil
}

func slugify(value string) string {
	value = strings.ToLower(clean(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if builder.Len() > 0 && !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "unknown"
	}
	return slug
}
