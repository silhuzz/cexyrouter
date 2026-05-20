package coinex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pedro/cex-router/pkg/types"
	"github.com/shopspring/decimal"
)

const (
	Slug           = "coinex"
	defaultBaseURL = "https://api.coinex.com"

	configPath = "/v2/assets/all-deposit-withdraw-config"

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

	var response configResponse
	if err := adapter.getJSON(ctx, configPath, &response); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch coinex config: %w", err)
	}
	if response.Code != 0 {
		return types.FetchResult{}, fmt.Errorf("fetch coinex config: code %d msg %q", response.Code, response.Message)
	}
	if len(response.Data) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch coinex config: empty data")
	}

	return snapshotsFromConfigs(response.Data, adapter.now().UTC())
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

func snapshotsFromConfigs(configs []assetConfig, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	for _, config := range configs {
		symbol := strings.ToUpper(strings.TrimSpace(config.Asset.Currency))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped asset with empty symbol")
			continue
		}
		if len(config.Chains) == 0 {
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no chains", symbol))
			continue
		}

		for _, chain := range config.Chains {
			rawChain := strings.TrimSpace(chain.Chain)
			if rawChain == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s chain with empty chain", symbol))
				continue
			}

			snapshot, err := snapshotFromChain(symbol, config.Asset, chain, rawChain, observedAt)
			if err != nil {
				return types.FetchResult{}, err
			}
			result.Snapshots = append(result.Snapshots, snapshot)
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("coinex response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromChain(symbol string, asset assetRecord, chain chainRecord, rawChain string, observedAt time.Time) (types.RailSnapshot, error) {
	withdrawMin, _, err := optionalDecimal(chain.MinWithdrawAmount)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s min_withdraw_amount: %w", symbol, rawChain, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(chain.WithdrawalFee)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdrawal_fee: %w", symbol, rawChain, err)
	}

	snapshot := types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             symbol,
		RawChainSymbol:       rawChain,
		RawChainName:         rawChain,
		RawNetworkID:         rawChain,
		DepositEnabled:       asset.DepositEnabled && chain.DepositEnabled,
		WithdrawEnabled:      asset.WithdrawEnabled && chain.WithdrawEnabled,
		DepositConfirmations: optionalInt(chain.SafeConfirmations),
		WithdrawMin:          withdrawMin,
		WithdrawFee:          withdrawFee,
		ObservedAt:           observedAt,
	}
	if hasWithdrawFee {
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	}
	return snapshot, nil
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

func optionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

type configResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    []assetConfig `json:"data"`
}

type assetConfig struct {
	Asset  assetRecord   `json:"asset"`
	Chains []chainRecord `json:"chains"`
}

type assetRecord struct {
	Currency        string `json:"ccy"`
	DepositEnabled  bool   `json:"deposit_enabled"`
	WithdrawEnabled bool   `json:"withdraw_enabled"`
}

type chainRecord struct {
	Chain             string `json:"chain"`
	MinDepositAmount  string `json:"min_deposit_amount"`
	MinWithdrawAmount string `json:"min_withdraw_amount"`
	DepositEnabled    bool   `json:"deposit_enabled"`
	WithdrawEnabled   bool   `json:"withdraw_enabled"`
	WithdrawalFee     string `json:"withdrawal_fee"`
	SafeConfirmations *int   `json:"safe_confirmations"`
}
