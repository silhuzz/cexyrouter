package bitmart

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
	Slug           = "bitmart"
	defaultBaseURL = "https://api-cloud.bitmart.com"

	currenciesPath = "/account/v1/currencies"

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
		return types.FetchResult{}, fmt.Errorf("fetch bitmart currencies: %w", err)
	}
	if response.Code != 1000 {
		return types.FetchResult{}, fmt.Errorf("fetch bitmart currencies: code %d msg %q", response.Code, response.Message)
	}
	if len(response.Data.Currencies) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch bitmart currencies: empty data")
	}

	return snapshotsFromCurrencies(response.Data.Currencies, adapter.now().UTC())
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
		Snapshots: make([]types.RailSnapshot, 0, len(currencies)),
		Complete:  true,
	}

	for _, currency := range currencies {
		network := strings.TrimSpace(currency.Network)
		symbol := coinSymbol(currency.Currency, network)
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped currency with empty symbol")
			continue
		}
		if network == "" {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with empty network", symbol))
			continue
		}

		snapshot, err := snapshotFromCurrency(symbol, network, currency, observedAt)
		if err != nil {
			return types.FetchResult{}, err
		}
		result.Snapshots = append(result.Snapshots, snapshot)
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("bitmart response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromCurrency(symbol string, network string, currency currencyRecord, observedAt time.Time) (types.RailSnapshot, error) {
	withdrawMin, _, err := optionalDecimal(currency.WithdrawMinSize)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_min: %w", symbol, network, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(currency.WithdrawFee)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_fee: %w", symbol, network, err)
	}

	snapshot := types.RailSnapshot{
		ExchangeSlug:    Slug,
		CoinSymbol:      symbol,
		CoinName:        coinName(currency.Name, network, symbol),
		RawChainSymbol:  network,
		RawChainName:    network,
		RawNetworkID:    network,
		DepositEnabled:  currency.DepositEnabled,
		WithdrawEnabled: currency.WithdrawEnabled,
		WithdrawMin:     withdrawMin,
		WithdrawFee:     withdrawFee,
		ObservedAt:      observedAt,
	}
	if hasWithdrawFee {
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	}
	return snapshot, nil
}

func coinSymbol(raw string, network string) string {
	raw = strings.TrimSpace(raw)
	network = strings.TrimSpace(network)
	if raw == "" {
		return ""
	}
	raw = trimNetworkSuffix(raw, network)
	switch strings.ToUpper(raw) {
	case "AVAX-C":
		raw = "AVAX"
	}
	return strings.ToUpper(raw)
}

func coinName(raw string, network string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	raw = trimNetworkSuffix(raw, network)
	if raw == "" {
		return fallback
	}
	return raw
}

func trimNetworkSuffix(raw string, network string) string {
	raw = strings.TrimSpace(raw)
	network = strings.TrimSpace(network)
	if raw == "" || network == "" || strings.EqualFold(raw, network) {
		return raw
	}
	suffix := "-" + network
	if len(raw) > len(suffix) && strings.EqualFold(raw[len(raw)-len(suffix):], suffix) {
		return strings.TrimSpace(raw[:len(raw)-len(suffix)])
	}
	return raw
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

type currenciesResponse struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    currenciesData `json:"data"`
}

type currenciesData struct {
	Currencies []currencyRecord `json:"currencies"`
}

type currencyRecord struct {
	Currency        string `json:"currency"`
	Name            string `json:"name"`
	Network         string `json:"network"`
	DepositEnabled  bool   `json:"deposit_enabled"`
	WithdrawEnabled bool   `json:"withdraw_enabled"`
	WithdrawMinSize string `json:"withdraw_minsize"`
	RechargeMinSize string `json:"recharge_minsize"`
	WithdrawFee     string `json:"withdraw_fee"`
}
