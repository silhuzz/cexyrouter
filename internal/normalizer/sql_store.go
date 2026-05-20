package normalizer

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type PgxQuerier interface {
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
	_, err := s.q.Exec(ctx, `
		INSERT INTO coin_aliases (
			exchange_id, raw_symbol, raw_name, coin_id, confidence, first_seen, last_seen
		)
		VALUES ($1, $2, $3, $4, 1, $5, $5)
		ON CONFLICT (exchange_id, raw_symbol, raw_name) DO UPDATE
		SET coin_id = EXCLUDED.coin_id,
		    confidence = LEAST(coin_aliases.confidence + 1, 3),
		    last_seen = EXCLUDED.last_seen
	`, upsert.ExchangeID, clean(upsert.RawSymbol), clean(upsert.RawName), upsert.CoinID, upsert.SeenAt)
	return err
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
	_, err := s.q.Exec(ctx, `
		INSERT INTO chain_aliases (
			exchange_id, raw_symbol, raw_name, raw_network_id, chain_id, confidence, first_seen, last_seen
		)
		VALUES ($1, $2, $3, $4, $5, 1, $6, $6)
		ON CONFLICT (exchange_id, raw_symbol, raw_name, raw_network_id) DO UPDATE
		SET chain_id = EXCLUDED.chain_id,
		    confidence = LEAST(chain_aliases.confidence + 1, 3),
		    last_seen = EXCLUDED.last_seen
	`, upsert.ExchangeID, clean(upsert.RawSymbol), clean(upsert.RawName), clean(upsert.RawNetworkID), upsert.ChainID, upsert.SeenAt)
	return err
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
