package normalizer

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pedro/cex-router/pkg/types"
	"github.com/shopspring/decimal"
)

func TestResolveCoinUpsertsAliasOncePerPoll(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.coinsBySymbol["USDT"] = CoinRef{ID: 10, Slug: "usdt", Symbol: "USDT", Name: "Tether USD"}

	resolver := NewPollResolver(store, WithNow(fixedNow))
	for i := 0; i < 2; i++ {
		coin, err := resolver.ResolveCoin(ctx, "binance", "USDT", "Tether USD")
		if err != nil {
			t.Fatalf("ResolveCoin attempt %d: %v", i+1, err)
		}
		if coin.ID != 10 {
			t.Fatalf("ResolveCoin attempt %d returned coin ID %d, want 10", i+1, coin.ID)
		}
	}

	if got := len(store.coinAliasUpserts); got != 1 {
		t.Fatalf("coin alias upserts = %d, want 1", got)
	}
	upsert := store.coinAliasUpserts[0]
	if upsert.ExchangeID != 1 || upsert.RawSymbol != "USDT" || upsert.RawName != "Tether USD" || upsert.CoinID != 10 {
		t.Fatalf("unexpected coin alias upsert: %+v", upsert)
	}
	if !upsert.SeenAt.Equal(fixedNow()) {
		t.Fatalf("SeenAt = %s, want %s", upsert.SeenAt, fixedNow())
	}
}

func TestResolveCoinCachesResolvedRefsPerPoll(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.coinsBySymbol["USDT"] = CoinRef{ID: 10, Slug: "usdt", Symbol: "USDT", Name: "Tether USD"}

	resolver := NewPollResolver(store)
	for i := 0; i < 3; i++ {
		coin, err := resolver.ResolveCoin(ctx, "binance", "USDT", "Tether USD")
		if err != nil {
			t.Fatalf("ResolveCoin attempt %d: %v", i+1, err)
		}
		if coin.ID != 10 {
			t.Fatalf("ResolveCoin attempt %d returned coin ID %d, want 10", i+1, coin.ID)
		}
	}

	if store.coinAliasFinds != 1 {
		t.Fatalf("coin alias lookups = %d, want 1", store.coinAliasFinds)
	}
	if store.coinCanonicalFinds != 1 {
		t.Fatalf("coin canonical lookups = %d, want 1", store.coinCanonicalFinds)
	}
}

func TestResolveCoinDedupeKeyIncludesRawName(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.coinsBySymbol["USDT"] = CoinRef{ID: 10, Slug: "usdt", Symbol: "USDT", Name: "Tether USD"}

	resolver := NewPollResolver(store)
	if _, err := resolver.ResolveCoin(ctx, "binance", "USDT", "Tether USD"); err != nil {
		t.Fatalf("ResolveCoin with first raw name: %v", err)
	}
	if _, err := resolver.ResolveCoin(ctx, "binance", "USDT", "Tether"); err != nil {
		t.Fatalf("ResolveCoin with second raw name: %v", err)
	}

	if got := len(store.coinAliasUpserts); got != 2 {
		t.Fatalf("coin alias upserts = %d, want 2 because raw_name is part of the key", got)
	}
}

func TestResolveChainUsesSynonymAndUpsertsAliasOncePerPoll(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()

	resolver := NewPollResolver(store, WithNow(fixedNow))
	for i := 0; i < 2; i++ {
		chain, err := resolver.ResolveChain(ctx, "binance", "TRC20", "TRON", "")
		if err != nil {
			t.Fatalf("ResolveChain attempt %d: %v", i+1, err)
		}
		if chain.ID != 30 {
			t.Fatalf("ResolveChain attempt %d returned chain ID %d, want 30", i+1, chain.ID)
		}
	}

	if got := len(store.chainAliasUpserts); got != 1 {
		t.Fatalf("chain alias upserts = %d, want 1", got)
	}
	upsert := store.chainAliasUpserts[0]
	if upsert.ExchangeID != 1 || upsert.RawSymbol != "TRC20" || upsert.RawName != "TRON" || upsert.RawNetworkID != "" || upsert.ChainID != 30 {
		t.Fatalf("unexpected chain alias upsert: %+v", upsert)
	}
}

func TestResolveChainCachesResolvedRefsPerPoll(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()

	resolver := NewPollResolver(store)
	for i := 0; i < 3; i++ {
		chain, err := resolver.ResolveChain(ctx, "binance", "TRC20", "", "")
		if err != nil {
			t.Fatalf("ResolveChain attempt %d: %v", i+1, err)
		}
		if chain.ID != 30 {
			t.Fatalf("ResolveChain attempt %d returned chain ID %d, want 30", i+1, chain.ID)
		}
	}

	if store.chainAliasFinds != 1 {
		t.Fatalf("chain alias lookups = %d, want 1", store.chainAliasFinds)
	}
	if store.chainCanonicalFinds != 1 {
		t.Fatalf("chain canonical lookups = %d, want 1", store.chainCanonicalFinds)
	}
	if store.chainSlugFinds != 1 {
		t.Fatalf("chain slug lookups = %d, want 1", store.chainSlugFinds)
	}
}

func TestResolveChainDedupeKeyIncludesNetworkID(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()

	resolver := NewPollResolver(store)
	if _, err := resolver.ResolveChain(ctx, "binance", "BASE", "Base", "base-mainnet"); err != nil {
		t.Fatalf("ResolveChain with first network ID: %v", err)
	}
	if _, err := resolver.ResolveChain(ctx, "binance", "BASE", "Base", "8453"); err != nil {
		t.Fatalf("ResolveChain with second network ID: %v", err)
	}

	if got := len(store.chainAliasUpserts); got != 2 {
		t.Fatalf("chain alias upserts = %d, want 2 because raw_network_id is part of the key", got)
	}
}

func TestNormalizePreservesDecimalPointers(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.coinAliases[coinAliasKey{exchangeID: 1, rawSymbol: "USDT", rawName: ""}] = CoinRef{ID: 10, Slug: "usdt", Symbol: "USDT", Name: "Tether USD"}
	store.chainAliases[chainAliasKey{exchangeID: 1, rawSymbol: "TRC20", rawName: "", rawNetworkID: ""}] = ChainRef{ID: 30, Slug: "tron", Symbol: "TRX", Name: "TRON"}

	min := decimal.RequireFromString("0.000000010000000001")
	fee := decimal.RequireFromString("1.230000000000000001")
	percent := decimal.RequireFromString("0.123456789123456789")

	resolver := NewPollResolver(store)
	result, err := resolver.Normalize(ctx, types.FetchResult{
		Complete: true,
		Notes:    []string{"demo"},
		Snapshots: []types.RailSnapshot{{
			ExchangeSlug:       "binance",
			CoinSymbol:         "USDT",
			RawChainSymbol:     "TRC20",
			WithdrawMin:        &min,
			WithdrawFee:        &fee,
			WithdrawFeePercent: &percent,
		}},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if len(result.Rails) != 1 {
		t.Fatalf("normalized rails = %d, want 1", len(result.Rails))
	}

	snapshot := result.Rails[0].Snapshot
	if snapshot.WithdrawMin != &min || snapshot.WithdrawFee != &fee || snapshot.WithdrawFeePercent != &percent {
		t.Fatalf("decimal pointers were not preserved")
	}
	if snapshot.WithdrawMin.String() != "0.000000010000000001" ||
		snapshot.WithdrawFee.String() != "1.230000000000000001" ||
		snapshot.WithdrawFeePercent.String() != "0.123456789123456789" {
		t.Fatalf("decimal values changed: min=%s fee=%s percent=%s", snapshot.WithdrawMin, snapshot.WithdrawFee, snapshot.WithdrawFeePercent)
	}
}

func TestNormalizeUsesVerifiedContractCoinWithoutPoisoningGenericAlias(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	store.coinsBySymbol["BTC"] = CoinRef{ID: 10, Slug: "btc", Symbol: "BTC", Name: "Bitcoin"}
	store.contractCoins[contractKey{chainID: 25, contractAddress: "0x7130d2a12b9bcbfae4f2634d864a1ee1ce3ead9c"}] = CoinRef{ID: 11, Slug: "btcb", Symbol: "BTCB", Name: "Binance Bitcoin"}

	resolver := NewPollResolver(store)
	result, err := resolver.Normalize(ctx, types.FetchResult{
		Complete: true,
		Snapshots: []types.RailSnapshot{{
			ExchangeSlug:    "binance",
			CoinSymbol:      "BTC",
			CoinName:        "Bitcoin",
			RawChainSymbol:  "BSC",
			RawChainName:    "BNB Smart Chain",
			RawNetworkID:    "BSC",
			ContractAddress: "0x7130D2A12B9BCBFAE4F2634D864A1EE1CE3EAD9C",
		}},
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got := result.Rails[0].CoinID; got != 11 {
		t.Fatalf("CoinID = %d, want verified contract coin 11", got)
	}
	if got := len(store.coinAliasUpserts); got != 0 {
		t.Fatalf("coin alias upserts = %d, want 0 for contract-specific mapping", got)
	}

	coin, err := resolver.ResolveCoin(ctx, "binance", "BTC", "Bitcoin")
	if err != nil {
		t.Fatalf("ResolveCoin: %v", err)
	}
	if coin.ID != 10 {
		t.Fatalf("generic BTC alias resolved to ID %d, want native BTC ID 10", coin.ID)
	}
}

type fakeStore struct {
	exchanges map[string]int64

	coinAliases   map[coinAliasKey]CoinRef
	coinsBySymbol map[string]CoinRef
	contractCoins map[contractKey]CoinRef
	coinsCreated  []CoinRef

	chainAliases   map[chainAliasKey]ChainRef
	chainsBySymbol map[string]ChainRef
	chainsBySlug   map[string]ChainRef
	chainsCreated  []ChainRef

	coinAliasUpserts  []CoinAliasUpsert
	chainAliasUpserts []ChainAliasUpsert

	coinAliasFinds      int
	coinCanonicalFinds  int
	chainAliasFinds     int
	chainCanonicalFinds int
	chainSlugFinds      int
}

type contractKey struct {
	chainID         int64
	contractAddress string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		exchanges: map[string]int64{
			"binance": 1,
		},
		coinAliases:   make(map[coinAliasKey]CoinRef),
		coinsBySymbol: make(map[string]CoinRef),
		contractCoins: make(map[contractKey]CoinRef),
		chainAliases:  make(map[chainAliasKey]ChainRef),
		chainsBySymbol: map[string]ChainRef{
			"TRX":  {ID: 30, Slug: "tron", Symbol: "TRX", Name: "TRON"},
			"BASE": {ID: 80, Slug: "base", Symbol: "BASE", Name: "Base"},
		},
		chainsBySlug: map[string]ChainRef{
			"ethereum": {ID: 20, Slug: "ethereum", Symbol: "ETH", Name: "Ethereum"},
			"bsc":      {ID: 25, Slug: "bsc", Symbol: "BSC", Name: "BNB Smart Chain"},
			"tron":     {ID: 30, Slug: "tron", Symbol: "TRX", Name: "TRON"},
			"polygon":  {ID: 40, Slug: "polygon", Symbol: "POL", Name: "Polygon"},
			"arbitrum": {ID: 50, Slug: "arbitrum", Symbol: "ARB", Name: "Arbitrum One"},
			"optimism": {ID: 60, Slug: "optimism", Symbol: "OP", Name: "Optimism"},
			"base":     {ID: 80, Slug: "base", Symbol: "BASE", Name: "Base"},
		},
	}
}

func (f *fakeStore) ExchangeID(_ context.Context, exchangeSlug string) (int64, error) {
	id, ok := f.exchanges[strings.ToLower(strings.TrimSpace(exchangeSlug))]
	if !ok {
		return 0, errors.New("exchange not found")
	}
	return id, nil
}

func (f *fakeStore) FindCoinAlias(_ context.Context, exchangeID int64, rawSymbol string, rawName string) (CoinRef, bool, error) {
	f.coinAliasFinds++
	coin, ok := f.coinAliases[coinAliasKey{exchangeID: exchangeID, rawSymbol: rawSymbol, rawName: rawName}]
	return coin, ok, nil
}

func (f *fakeStore) FindCoinByContract(_ context.Context, chainID int64, contractAddress string) (CoinRef, bool, error) {
	coin, ok := f.contractCoins[contractKey{chainID: chainID, contractAddress: normalizeContractAddress(contractAddress)}]
	return coin, ok, nil
}

func (f *fakeStore) FindCoinCanonical(_ context.Context, symbol string, name string) (CoinRef, bool, error) {
	f.coinCanonicalFinds++
	if coin, ok := f.coinsBySymbol[strings.ToUpper(symbol)]; ok {
		return coin, true, nil
	}
	for _, coin := range f.coinsBySymbol {
		if strings.EqualFold(coin.Name, name) && name != "" {
			return coin, true, nil
		}
	}
	return CoinRef{}, false, nil
}

func (f *fakeStore) InsertCoin(_ context.Context, symbol string, name string) (CoinRef, error) {
	coin := CoinRef{ID: int64(100 + len(f.coinsCreated)), Slug: strings.ToLower(symbol), Symbol: symbol, Name: name}
	f.coinsCreated = append(f.coinsCreated, coin)
	f.coinsBySymbol[strings.ToUpper(symbol)] = coin
	return coin, nil
}

func (f *fakeStore) UpsertCoinAlias(_ context.Context, upsert CoinAliasUpsert) error {
	f.coinAliasUpserts = append(f.coinAliasUpserts, upsert)
	f.coinAliases[coinAliasKey{exchangeID: upsert.ExchangeID, rawSymbol: upsert.RawSymbol, rawName: upsert.RawName}] = CoinRef{ID: upsert.CoinID}
	return nil
}

func (f *fakeStore) FindChainAlias(_ context.Context, exchangeID int64, rawSymbol string, rawName string, rawNetworkID string) (ChainRef, bool, error) {
	f.chainAliasFinds++
	chain, ok := f.chainAliases[chainAliasKey{exchangeID: exchangeID, rawSymbol: rawSymbol, rawName: rawName, rawNetworkID: rawNetworkID}]
	return chain, ok, nil
}

func (f *fakeStore) FindChainCanonical(_ context.Context, symbol string, name string) (ChainRef, bool, error) {
	f.chainCanonicalFinds++
	if chain, ok := f.chainsBySymbol[strings.ToUpper(symbol)]; ok {
		return chain, true, nil
	}
	for _, chain := range f.chainsBySlug {
		if strings.EqualFold(chain.Name, name) && name != "" {
			return chain, true, nil
		}
	}
	return ChainRef{}, false, nil
}

func (f *fakeStore) FindChainBySlug(_ context.Context, slug string) (ChainRef, bool, error) {
	f.chainSlugFinds++
	chain, ok := f.chainsBySlug[slug]
	return chain, ok, nil
}

func (f *fakeStore) InsertChain(_ context.Context, symbol string, name string) (ChainRef, error) {
	chain := ChainRef{ID: int64(200 + len(f.chainsCreated)), Slug: strings.ToLower(symbol), Symbol: symbol, Name: name}
	f.chainsCreated = append(f.chainsCreated, chain)
	f.chainsBySymbol[strings.ToUpper(symbol)] = chain
	return chain, nil
}

func (f *fakeStore) UpsertChainAlias(_ context.Context, upsert ChainAliasUpsert) error {
	f.chainAliasUpserts = append(f.chainAliasUpserts, upsert)
	f.chainAliases[chainAliasKey{exchangeID: upsert.ExchangeID, rawSymbol: upsert.RawSymbol, rawName: upsert.RawName, rawNetworkID: upsert.RawNetworkID}] = ChainRef{ID: upsert.ChainID}
	return nil
}

func fixedNow() time.Time {
	return time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC)
}
