package whitebit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

const (
	Slug           = "whitebit"
	defaultBaseURL = "https://whitebit.com"

	assetsPath = "/api/v4/public/assets"

	maxResponseBytes = 20 << 20
)

var _ types.Adapter = (*Adapter)(nil)

type Adapter struct {
	baseURL string
	client  *http.Client
	now     func() time.Time
}

type Option func(*Adapter)

func New(opts ...Option) *Adapter {
	adapter := &Adapter{
		baseURL: defaultBaseURL,
		client:  http.DefaultClient,
		now:     time.Now,
	}
	for _, opt := range opts {
		opt(adapter)
	}
	if adapter.client == nil {
		adapter.client = http.DefaultClient
	}
	if adapter.now == nil {
		adapter.now = time.Now
	}
	adapter.baseURL = strings.TrimRight(adapter.baseURL, "/")
	return adapter
}

func WithBaseURL(baseURL string) Option {
	return func(adapter *Adapter) {
		adapter.baseURL = baseURL
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(adapter *Adapter) {
		adapter.client = client
	}
}

func WithNow(now func() time.Time) Option {
	return func(adapter *Adapter) {
		if now != nil {
			adapter.now = now
		}
	}
}

func (a *Adapter) Slug() string {
	return Slug
}

func (a *Adapter) FetchRails(ctx context.Context) (types.FetchResult, error) {
	adapter := a
	if adapter == nil {
		adapter = New()
	}

	var assets map[string]assetRecord
	if err := adapter.getJSON(ctx, assetsPath, &assets); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch whitebit assets: %w", err)
	}
	if len(assets) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch whitebit assets: empty data")
	}

	return snapshotsFromAssets(assets, adapter.now().UTC())
}

func (a *Adapter) getJSON(ctx context.Context, path string, dest any) error {
	if a.baseURL == "" {
		a.baseURL = defaultBaseURL
	}
	if a.client == nil {
		a.client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cex-router/1.0")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("response exceeded %d bytes", maxResponseBytes)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return err
	}
	return nil
}

func snapshotsFromAssets(assets map[string]assetRecord, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	symbols := make([]string, 0, len(assets))
	for symbol := range assets {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	for _, rawSymbol := range symbols {
		asset := assets[rawSymbol]
		symbol := strings.ToUpper(strings.TrimSpace(rawSymbol))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped asset with empty symbol")
			continue
		}

		networks := assetNetworks(asset)
		if len(networks) == 0 {
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no networks", symbol))
			continue
		}

		for _, network := range networks {
			snapshot, err := snapshotFromNetwork(symbol, asset, network, observedAt)
			if err != nil {
				return types.FetchResult{}, err
			}
			result.Snapshots = append(result.Snapshots, snapshot)
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("whitebit response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromNetwork(symbol string, asset assetRecord, network string, observedAt time.Time) (types.RailSnapshot, error) {
	withdrawMin, _, err := optionalDecimal(firstNonEmpty(asset.Limits.Withdraw[network].Min, asset.MinWithdraw))
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_min: %w", symbol, network, err)
	}

	return types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             firstNonEmpty(asset.Name, symbol),
		RawChainSymbol:       network,
		RawChainName:         network,
		RawNetworkID:         network,
		DepositEnabled:       asset.CanDeposit && contains(asset.Networks.Deposits, network),
		WithdrawEnabled:      asset.CanWithdraw && contains(asset.Networks.Withdraws, network),
		DepositConfirmations: optionalInt(asset.Confirmations[network]),
		WithdrawMin:          withdrawMin,
		ObservedAt:           observedAt,
	}, nil
}

func assetNetworks(asset assetRecord) []string {
	seen := make(map[string]struct{})
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	add(asset.Networks.Deposits...)
	add(asset.Networks.Withdraws...)
	add(asset.Networks.Default)
	for network := range asset.Confirmations {
		add(network)
	}

	networks := make([]string, 0, len(seen))
	for network := range seen {
		networks = append(networks, network)
	}
	sort.Strings(networks)
	return networks
}

func optionalDecimal(raw string) (*decimal.Decimal, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return nil, false, err
	}
	return &value, true, nil
}

func optionalInt(value int) *int {
	copied := value
	return &copied
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(needle)) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type assetRecord struct {
	Name          string         `json:"name"`
	CanWithdraw   bool           `json:"can_withdraw"`
	CanDeposit    bool           `json:"can_deposit"`
	MinWithdraw   string         `json:"min_withdraw"`
	Networks      networksRecord `json:"networks"`
	Confirmations map[string]int `json:"confirmations"`
	Limits        limitsRecord   `json:"limits"`
}

type networksRecord struct {
	Deposits  []string `json:"deposits"`
	Withdraws []string `json:"withdraws"`
	Default   string   `json:"default"`
}

type limitsRecord struct {
	Withdraw map[string]limitRecord `json:"withdraw"`
}

type limitRecord struct {
	Min string `json:"min"`
}
