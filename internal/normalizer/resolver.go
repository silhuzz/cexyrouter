package normalizer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pedro/cex-router/internal/chains"
	"github.com/pedro/cex-router/pkg/types"
)

var (
	ErrMissingStore      = errors.New("normalizer: missing alias store")
	ErrMissingExchange   = errors.New("normalizer: missing exchange slug")
	ErrMissingCoinInput  = errors.New("normalizer: missing coin symbol")
	ErrMissingChainInput = errors.New("normalizer: missing chain symbol or name")
)

type CoinRef struct {
	ID     int64
	Slug   string
	Symbol string
	Name   string
}

type ChainRef struct {
	ID     int64
	Slug   string
	Symbol string
	Name   string
}

type CoinAliasUpsert struct {
	ExchangeID int64
	RawSymbol  string
	RawName    string
	CoinID     int64
	SeenAt     time.Time
}

type ChainAliasUpsert struct {
	ExchangeID   int64
	RawSymbol    string
	RawName      string
	RawNetworkID string
	ChainID      int64
	SeenAt       time.Time
}

type AliasStore interface {
	ExchangeID(ctx context.Context, exchangeSlug string) (int64, error)

	FindCoinAlias(ctx context.Context, exchangeID int64, rawSymbol string, rawName string) (CoinRef, bool, error)
	FindCoinByContract(ctx context.Context, chainID int64, contractAddress string) (CoinRef, bool, error)
	FindCoinCanonical(ctx context.Context, symbol string, name string) (CoinRef, bool, error)
	InsertCoin(ctx context.Context, symbol string, name string) (CoinRef, error)
	UpsertCoinAlias(ctx context.Context, upsert CoinAliasUpsert) error

	FindChainAlias(ctx context.Context, exchangeID int64, rawSymbol string, rawName string, rawNetworkID string) (ChainRef, bool, error)
	FindChainCanonical(ctx context.Context, symbol string, name string) (ChainRef, bool, error)
	FindChainBySlug(ctx context.Context, slug string) (ChainRef, bool, error)
	InsertChain(ctx context.Context, symbol string, name string) (ChainRef, error)
	UpsertChainAlias(ctx context.Context, upsert ChainAliasUpsert) error
}

type NormalizedFetchResult struct {
	Rails    []NormalizedRail
	Complete bool
	Notes    []string
}

type NormalizedRail struct {
	ExchangeID int64
	CoinID     int64
	ChainID    int64
	Snapshot   types.RailSnapshot
}

type Option func(*PollResolver)

type PollResolver struct {
	store AliasStore
	now   func() time.Time

	exchangeIDs      map[string]int64
	coinRefs         map[coinResolutionKey]CoinRef
	chainRefs        map[chainAliasKey]ChainRef
	seenCoinAliases  map[coinAliasKey]struct{}
	seenChainAliases map[chainAliasKey]struct{}
}

type coinAliasKey struct {
	exchangeID int64
	rawSymbol  string
	rawName    string
}

type coinResolutionKey struct {
	exchangeID      int64
	rawSymbol       string
	rawName         string
	chainID         int64
	contractAddress string
}

type chainAliasKey struct {
	exchangeID   int64
	rawSymbol    string
	rawName      string
	rawNetworkID string
}

func NewPollResolver(store AliasStore, opts ...Option) *PollResolver {
	resolver := &PollResolver{
		store:            store,
		now:              time.Now,
		exchangeIDs:      make(map[string]int64),
		coinRefs:         make(map[coinResolutionKey]CoinRef),
		chainRefs:        make(map[chainAliasKey]ChainRef),
		seenCoinAliases:  make(map[coinAliasKey]struct{}),
		seenChainAliases: make(map[chainAliasKey]struct{}),
	}
	for _, opt := range opts {
		opt(resolver)
	}
	return resolver
}

func WithNow(now func() time.Time) Option {
	return func(resolver *PollResolver) {
		if now != nil {
			resolver.now = now
		}
	}
}

func ResolveCoin(ctx context.Context, store AliasStore, exchangeSlug string, rawSymbol string, rawName string) (CoinRef, error) {
	return NewPollResolver(store).ResolveCoin(ctx, exchangeSlug, rawSymbol, rawName)
}

func ResolveChain(ctx context.Context, store AliasStore, exchangeSlug string, rawSymbol string, rawName string, rawNetworkID string) (ChainRef, error) {
	return NewPollResolver(store).ResolveChain(ctx, exchangeSlug, rawSymbol, rawName, rawNetworkID)
}

func (r *PollResolver) Normalize(ctx context.Context, result types.FetchResult) (NormalizedFetchResult, error) {
	normalized := NormalizedFetchResult{
		Rails:    make([]NormalizedRail, 0, len(result.Snapshots)),
		Complete: result.Complete,
		Notes:    append([]string(nil), result.Notes...),
	}

	for i, snapshot := range result.Snapshots {
		exchangeID, err := r.exchangeID(ctx, snapshot.ExchangeSlug)
		if err != nil {
			return NormalizedFetchResult{}, fmt.Errorf("snapshot %d exchange: %w", i, err)
		}
		chain, err := r.resolveChain(ctx, exchangeID, snapshot.RawChainSymbol, snapshot.RawChainName, snapshot.RawNetworkID)
		if err != nil {
			return NormalizedFetchResult{}, fmt.Errorf("snapshot %d chain: %w", i, err)
		}
		coin, err := r.resolveCoin(ctx, exchangeID, snapshot.CoinSymbol, snapshot.CoinName, chain.ID, snapshot.ContractAddress)
		if err != nil {
			return NormalizedFetchResult{}, fmt.Errorf("snapshot %d coin: %w", i, err)
		}

		normalized.Rails = append(normalized.Rails, NormalizedRail{
			ExchangeID: exchangeID,
			CoinID:     coin.ID,
			ChainID:    chain.ID,
			Snapshot:   snapshot,
		})
	}

	return normalized, nil
}

func (r *PollResolver) ResolveCoin(ctx context.Context, exchangeSlug string, rawSymbol string, rawName string) (CoinRef, error) {
	exchangeID, err := r.exchangeID(ctx, exchangeSlug)
	if err != nil {
		return CoinRef{}, err
	}
	return r.resolveCoin(ctx, exchangeID, rawSymbol, rawName, 0, "")
}

func (r *PollResolver) ResolveChain(ctx context.Context, exchangeSlug string, rawSymbol string, rawName string, rawNetworkID string) (ChainRef, error) {
	exchangeID, err := r.exchangeID(ctx, exchangeSlug)
	if err != nil {
		return ChainRef{}, err
	}
	return r.resolveChain(ctx, exchangeID, rawSymbol, rawName, rawNetworkID)
}

func (r *PollResolver) resolveCoin(ctx context.Context, exchangeID int64, symbol string, name string, chainID int64, contractAddress string) (CoinRef, error) {
	if r.store == nil {
		return CoinRef{}, ErrMissingStore
	}

	rawSymbol := clean(symbol)
	rawName := clean(name)
	normalizedContract := normalizeContractAddress(contractAddress)
	if rawSymbol == "" {
		return CoinRef{}, ErrMissingCoinInput
	}

	key := coinResolutionKey{
		exchangeID:      exchangeID,
		rawSymbol:       rawSymbol,
		rawName:         rawName,
		chainID:         chainID,
		contractAddress: normalizedContract,
	}
	if coin, ok := r.coinRefs[key]; ok {
		return coin, nil
	}

	if normalizedContract != "" && chainID != 0 {
		coin, ok, err := r.store.FindCoinByContract(ctx, chainID, normalizedContract)
		if err != nil {
			return CoinRef{}, fmt.Errorf("find coin by contract: %w", err)
		}
		if ok {
			r.coinRefs[key] = coin
			return coin, nil
		}
	}

	coin, ok, err := r.store.FindCoinAlias(ctx, exchangeID, rawSymbol, rawName)
	if err != nil {
		return CoinRef{}, fmt.Errorf("find coin alias: %w", err)
	}
	if !ok {
		coin, ok, err = r.store.FindCoinCanonical(ctx, rawSymbol, rawName)
		if err != nil {
			return CoinRef{}, fmt.Errorf("find canonical coin: %w", err)
		}
	}
	if !ok {
		coin, err = r.store.InsertCoin(ctx, rawSymbol, rawName)
		if err != nil {
			return CoinRef{}, fmt.Errorf("insert coin: %w", err)
		}
	}

	upsert := CoinAliasUpsert{
		ExchangeID: exchangeID,
		RawSymbol:  rawSymbol,
		RawName:    rawName,
		CoinID:     coin.ID,
		SeenAt:     r.now(),
	}
	if err := r.upsertCoinAliasOnce(ctx, upsert); err != nil {
		return CoinRef{}, err
	}
	r.coinRefs[key] = coin
	return coin, nil
}

func (r *PollResolver) resolveChain(ctx context.Context, exchangeID int64, symbol string, name string, networkID string) (ChainRef, error) {
	if r.store == nil {
		return ChainRef{}, ErrMissingStore
	}

	rawSymbol := clean(symbol)
	rawName := clean(name)
	rawNetworkID := clean(networkID)
	if rawSymbol == "" && rawName == "" {
		return ChainRef{}, ErrMissingChainInput
	}

	key := chainAliasKey{
		exchangeID:   exchangeID,
		rawSymbol:    rawSymbol,
		rawName:      rawName,
		rawNetworkID: rawNetworkID,
	}
	if chain, ok := r.chainRefs[key]; ok {
		return chain, nil
	}

	chain, ok, err := r.store.FindChainAlias(ctx, exchangeID, rawSymbol, rawName, rawNetworkID)
	if err != nil {
		return ChainRef{}, fmt.Errorf("find chain alias: %w", err)
	}
	if !ok {
		chain, ok, err = r.store.FindChainCanonical(ctx, rawSymbol, rawName)
		if err != nil {
			return ChainRef{}, fmt.Errorf("find canonical chain: %w", err)
		}
	}
	if !ok {
		if slug, synonymOK := chains.CanonicalSlug(rawSymbol, rawName); synonymOK {
			chain, ok, err = r.store.FindChainBySlug(ctx, slug)
			if err != nil {
				return ChainRef{}, fmt.Errorf("find synonym chain %q: %w", slug, err)
			}
		}
	}
	if !ok {
		chain, err = r.store.InsertChain(ctx, firstNonEmpty(rawSymbol, rawName), rawName)
		if err != nil {
			return ChainRef{}, fmt.Errorf("insert chain: %w", err)
		}
	}

	upsert := ChainAliasUpsert{
		ExchangeID:   exchangeID,
		RawSymbol:    rawSymbol,
		RawName:      rawName,
		RawNetworkID: rawNetworkID,
		ChainID:      chain.ID,
		SeenAt:       r.now(),
	}
	if err := r.upsertChainAliasOnce(ctx, upsert); err != nil {
		return ChainRef{}, err
	}
	r.chainRefs[key] = chain
	return chain, nil
}

func (r *PollResolver) exchangeID(ctx context.Context, exchangeSlug string) (int64, error) {
	if r.store == nil {
		return 0, ErrMissingStore
	}
	slug := strings.ToLower(clean(exchangeSlug))
	if slug == "" {
		return 0, ErrMissingExchange
	}
	if id, ok := r.exchangeIDs[slug]; ok {
		return id, nil
	}
	id, err := r.store.ExchangeID(ctx, slug)
	if err != nil {
		return 0, fmt.Errorf("lookup exchange %q: %w", slug, err)
	}
	r.exchangeIDs[slug] = id
	return id, nil
}

func (r *PollResolver) upsertCoinAliasOnce(ctx context.Context, upsert CoinAliasUpsert) error {
	key := coinAliasKey{
		exchangeID: upsert.ExchangeID,
		rawSymbol:  upsert.RawSymbol,
		rawName:    upsert.RawName,
	}
	if _, ok := r.seenCoinAliases[key]; ok {
		return nil
	}
	if err := r.store.UpsertCoinAlias(ctx, upsert); err != nil {
		return fmt.Errorf("upsert coin alias: %w", err)
	}
	r.seenCoinAliases[key] = struct{}{}
	return nil
}

func (r *PollResolver) upsertChainAliasOnce(ctx context.Context, upsert ChainAliasUpsert) error {
	key := chainAliasKey{
		exchangeID:   upsert.ExchangeID,
		rawSymbol:    upsert.RawSymbol,
		rawName:      upsert.RawName,
		rawNetworkID: upsert.RawNetworkID,
	}
	if _, ok := r.seenChainAliases[key]; ok {
		return nil
	}
	if err := r.store.UpsertChainAlias(ctx, upsert); err != nil {
		return fmt.Errorf("upsert chain alias: %w", err)
	}
	r.seenChainAliases[key] = struct{}{}
	return nil
}

func clean(value string) string {
	return strings.TrimSpace(value)
}

func normalizeContractAddress(value string) string {
	value = clean(value)
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		return strings.ToLower(value)
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
