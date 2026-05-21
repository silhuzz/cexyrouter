package kucoin

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
	Slug           = "kucoin"
	defaultBaseURL = "https://api.kucoin.com"

	currenciesPath = "/api/ua/v1/asset/currencies"

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

	var response currenciesResponse
	if err := adapter.getJSON(ctx, currenciesPath, &response); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch kucoin currencies: %w", err)
	}
	if response.Code != "200000" {
		return types.FetchResult{}, fmt.Errorf("fetch kucoin currencies: code %q", response.Code)
	}
	if len(response.Data) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch kucoin currencies: empty data")
	}

	return snapshotsFromCurrencies(response.Data, adapter.now().UTC())
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

func snapshotsFromCurrencies(currencies []currencyRecord, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	for _, currency := range currencies {
		symbol := strings.ToUpper(strings.TrimSpace(currency.Currency))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped currency with empty symbol")
			continue
		}
		if len(currency.Items) == 0 {
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no chain items", symbol))
			continue
		}

		for _, item := range currency.Items {
			rawChain := strings.TrimSpace(item.ChainName)
			rawNetworkID := strings.TrimSpace(item.ChainID)
			if rawChain == "" && rawNetworkID == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s chain with empty identifiers", symbol))
				continue
			}

			snapshot, err := snapshotFromItem(currency, symbol, item, observedAt)
			if err != nil {
				return types.FetchResult{}, err
			}
			result.Snapshots = append(result.Snapshots, snapshot)
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("kucoin response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromItem(currency currencyRecord, symbol string, item chainItem, observedAt time.Time) (types.RailSnapshot, error) {
	rawChain := firstNonEmpty(strings.TrimSpace(item.ChainName), strings.TrimSpace(item.ChainID))
	withdrawMin, _, err := optionalDecimal(item.MinWithdrawSize)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s min_withdraw_size: %w", symbol, rawChain, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(item.MinWithdrawFee)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s min_withdraw_fee: %w", symbol, rawChain, err)
	}
	withdrawFeeRate, hasWithdrawFeeRate, err := optionalNonZeroDecimal(item.WithdrawFeeRate)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_fee_rate: %w", symbol, rawChain, err)
	}
	depositConfirmations := optionalIntFromValue(item.Confirms)

	snapshot := types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             firstNonEmpty(strings.TrimSpace(currency.FullName), strings.TrimSpace(currency.Name)),
		RawChainSymbol:       rawChain,
		RawChainName:         rawChain,
		RawNetworkID:         strings.TrimSpace(item.ChainID),
		ContractAddress:      strings.TrimSpace(item.ContractAddress),
		DepositEnabled:       item.IsDepositEnabled,
		WithdrawEnabled:      item.IsWithdrawEnabled,
		DepositConfirmations: depositConfirmations,
		WithdrawMin:          withdrawMin,
		WithdrawFee:          withdrawFee,
		ObservedAt:           observedAt,
	}
	if snapshot.RawNetworkID == "" {
		snapshot.RawNetworkID = rawChain
	}

	switch {
	case hasWithdrawFee && hasWithdrawFeeRate:
		snapshot.WithdrawFeeType = types.FeeTypeHybrid
		snapshot.WithdrawFeePercent = withdrawFeeRate
	case hasWithdrawFee:
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	case hasWithdrawFeeRate:
		snapshot.WithdrawFeeType = types.FeeTypePercent
		snapshot.WithdrawFeePercent = withdrawFeeRate
	}

	return snapshot, nil
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

func optionalIntFromValue(raw any) *int {
	switch value := raw.(type) {
	case float64:
		parsed := int(value)
		return &parsed
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil
		}
		return &parsed
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type currenciesResponse struct {
	Code string           `json:"code"`
	Data []currencyRecord `json:"data"`
}

type currencyRecord struct {
	Currency string      `json:"currency"`
	Name     string      `json:"name"`
	FullName string      `json:"fullName"`
	Items    []chainItem `json:"items"`
}

type chainItem struct {
	ChainName         string `json:"chainName"`
	MinWithdrawSize   string `json:"minWithdrawSize"`
	MinDepositSize    string `json:"minDepositSize"`
	WithdrawFeeRate   string `json:"withdrawFeeRate"`
	MinWithdrawFee    string `json:"minWithdrawFee"`
	IsWithdrawEnabled bool   `json:"isWithdrawEnabled"`
	IsDepositEnabled  bool   `json:"isDepositEnabled"`
	Confirms          any    `json:"confirms"`
	PreConfirms       any    `json:"preConfirms"`
	ContractAddress   string `json:"contractAddress"`
	ChainID           string `json:"chainId"`
}
