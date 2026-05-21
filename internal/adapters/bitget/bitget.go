package bitget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

const (
	Slug           = "bitget"
	defaultBaseURL = "https://api.bitget.com"

	coinsPath = "/api/v2/spot/public/coins"

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

	var response coinsResponse
	if err := adapter.getJSON(ctx, coinsPath, &response); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch bitget coins: %w", err)
	}
	if response.Code != "00000" {
		return types.FetchResult{}, fmt.Errorf("fetch bitget coins: code %q msg %q", response.Code, response.Message)
	}
	if len(response.Data) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch bitget coins: empty data")
	}

	return snapshotsFromCoins(response.Data, adapter.now().UTC())
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

func snapshotsFromCoins(coins []coinRecord, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	for _, coin := range coins {
		symbol := strings.ToUpper(strings.TrimSpace(coin.Coin))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped coin with empty symbol")
			continue
		}
		if len(coin.Chains) == 0 {
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no chains", symbol))
			continue
		}

		for _, chain := range coin.Chains {
			rawChain := strings.TrimSpace(chain.Chain)
			if rawChain == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s chain with empty name", symbol))
				continue
			}

			snapshot, notes, err := snapshotFromChain(coin, symbol, chain, rawChain, observedAt)
			if err != nil {
				return types.FetchResult{}, err
			}
			if len(notes) > 0 {
				result.Complete = false
				result.Notes = append(result.Notes, notes...)
			}
			result.Snapshots = append(result.Snapshots, snapshot)
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("bitget response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromChain(coin coinRecord, symbol string, chain chainRecord, rawChain string, observedAt time.Time) (types.RailSnapshot, []string, error) {
	withdrawMin, _, err := optionalDecimal(chain.MinWithdrawAmount)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s min_withdraw_amount: %w", symbol, rawChain, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(chain.WithdrawFee)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s withdraw_fee: %w", symbol, rawChain, err)
	}
	extraWithdrawFee, hasExtraWithdrawFee, err := optionalNonZeroDecimal(chain.ExtraWithdrawFee)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s extra_withdraw_fee: %w", symbol, rawChain, err)
	}
	depositConfirmations, err := optionalInt(chain.DepositConfirm)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s deposit_confirm: %w", symbol, rawChain, err)
	}

	depositEnabled, depositOK := parseBoolString(chain.Rechargeable)
	withdrawEnabled, withdrawOK := parseBoolString(chain.Withdrawable)

	var notes []string
	if !depositOK {
		notes = append(notes, fmt.Sprintf("unexpected deposit status for %s %s: %q", symbol, rawChain, chain.Rechargeable))
	}
	if !withdrawOK {
		notes = append(notes, fmt.Sprintf("unexpected withdraw status for %s %s: %q", symbol, rawChain, chain.Withdrawable))
	}

	snapshot := types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             strings.TrimSpace(coin.Name),
		RawChainSymbol:       rawChain,
		RawChainName:         rawChain,
		RawNetworkID:         rawChain,
		ContractAddress:      strings.TrimSpace(chain.ContractAddress),
		DepositEnabled:       depositEnabled,
		WithdrawEnabled:      withdrawEnabled,
		DepositConfirmations: depositConfirmations,
		WithdrawMin:          withdrawMin,
		WithdrawFee:          withdrawFee,
		ObservedAt:           observedAt,
	}

	switch {
	case hasWithdrawFee && hasExtraWithdrawFee:
		snapshot.WithdrawFeeType = types.FeeTypeHybrid
		snapshot.WithdrawFeePercent = extraWithdrawFee
	case hasWithdrawFee:
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	case hasExtraWithdrawFee:
		snapshot.WithdrawFeeType = types.FeeTypePercent
		snapshot.WithdrawFeePercent = extraWithdrawFee
	}

	return snapshot, notes, nil
}

func optionalDecimal(raw string) (*decimal.Decimal, bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, false, nil
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return nil, false, err
	}
	return &parsed, true, nil
}

func optionalNonZeroDecimal(raw string) (*decimal.Decimal, bool, error) {
	parsed, ok, err := optionalDecimal(raw)
	if err != nil || !ok {
		return parsed, ok, err
	}
	if parsed.Equal(decimal.Zero) {
		return nil, false, nil
	}
	percent := parsed.Mul(decimal.NewFromInt(100))
	return &percent, true, nil
}

func optionalInt(raw string) (*int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseBoolString(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

type coinsResponse struct {
	Code    string       `json:"code"`
	Message string       `json:"msg"`
	Data    []coinRecord `json:"data"`
}

type coinRecord struct {
	Coin   string        `json:"coin"`
	Name   string        `json:"name"`
	Chains []chainRecord `json:"chains"`
}

type chainRecord struct {
	Chain             string `json:"chain"`
	Withdrawable      string `json:"withdrawable"`
	Rechargeable      string `json:"rechargeable"`
	WithdrawFee       string `json:"withdrawFee"`
	ExtraWithdrawFee  string `json:"extraWithdrawFee"`
	DepositConfirm    string `json:"depositConfirm"`
	MinDepositAmount  string `json:"minDepositAmount"`
	MinWithdrawAmount string `json:"minWithdrawAmount"`
	Congestion        string `json:"congestion"`
	ContractAddress   string `json:"contractAddress"`
}
