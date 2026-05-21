package htx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

const (
	Slug           = "htx"
	defaultBaseURL = "https://api.huobi.pro"

	currenciesPath = "/v2/reference/currencies?authorizedUser=false"

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
		return types.FetchResult{}, fmt.Errorf("fetch htx currencies: %w", err)
	}
	if response.Code != 200 {
		return types.FetchResult{}, fmt.Errorf("fetch htx currencies: code %d msg %q", response.Code, response.Message)
	}
	if len(response.Data) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch htx currencies: empty data")
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
		if len(currency.Chains) == 0 {
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no chains", symbol))
			continue
		}

		for _, chain := range currency.Chains {
			rawNetworkID := strings.TrimSpace(chain.Chain)
			rawChain := firstNonEmpty(chain.BaseChain, chain.DisplayName, chain.BaseChainProtocol, rawNetworkID)
			if rawChain == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s chain with empty chain id", symbol))
				continue
			}

			coinSymbol, coinName := coinIdentity(symbol, chain)
			snapshot, notes, err := snapshotFromChain(coinSymbol, coinName, currency, chain, rawChain, rawNetworkID, observedAt)
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
		return types.FetchResult{}, fmt.Errorf("htx response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromChain(symbol string, coinName string, currency currencyRecord, chain chainRecord, rawChain string, rawNetworkID string, observedAt time.Time) (types.RailSnapshot, []string, error) {
	withdrawMin, _, err := optionalDecimal(chain.MinWithdrawAmount)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s min_withdraw_amount: %w", symbol, rawChain, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(chain.TransactFeeWithdraw)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s withdraw_fee: %w", symbol, rawChain, err)
	}
	depositConfirmations := optionalInt(chain.NumConfirmations)

	depositEnabled, depositOK := allowedStatus(chain.DepositStatus)
	withdrawEnabled, withdrawOK := allowedStatus(chain.WithdrawStatus)

	var notes []string
	if !depositOK {
		notes = append(notes, fmt.Sprintf("unexpected deposit status for %s %s: %q", symbol, rawChain, chain.DepositStatus))
	}
	if !withdrawOK {
		notes = append(notes, fmt.Sprintf("unexpected withdraw status for %s %s: %q", symbol, rawChain, chain.WithdrawStatus))
	}

	snapshot := types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             firstNonEmpty(coinName, symbol),
		RawChainSymbol:       rawChain,
		RawChainName:         firstNonEmpty(chain.FullName, chain.DisplayName, rawChain),
		RawNetworkID:         firstNonEmpty(rawNetworkID, rawChain),
		DepositEnabled:       depositEnabled,
		WithdrawEnabled:      withdrawEnabled,
		DepositConfirmations: depositConfirmations,
		WithdrawMin:          withdrawMin,
		WithdrawFee:          withdrawFee,
		ObservedAt:           observedAt,
	}
	if hasWithdrawFee {
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	}
	if strings.EqualFold(currency.InstStatus, "delisted") {
		snapshot.DepositEnabled = false
		snapshot.WithdrawEnabled = false
	}
	return snapshot, notes, nil
}

func coinIdentity(symbol string, chain chainRecord) (string, string) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol != "BTC" {
		return symbol, symbol
	}

	for _, value := range []string{chain.FullName, chain.DisplayName, chain.Chain} {
		normalized := strings.ToUpper(strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.TrimSpace(value)))
		switch {
		case strings.Contains(normalized, "CBBTC"):
			return "CBBTC", "Coinbase Wrapped BTC"
		case strings.Contains(normalized, "TBTC"):
			return "TBTC", "tBTC"
		case strings.Contains(normalized, "WBTC"):
			return "WBTC", "Wrapped Bitcoin"
		}
	}
	return "BTC", "Bitcoin"
}

func allowedStatus(status string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "allowed":
		return true, true
	case "prohibited":
		return false, true
	default:
		return false, false
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type currenciesResponse struct {
	Code    int              `json:"code"`
	Message string           `json:"message"`
	Data    []currencyRecord `json:"data"`
}

type currencyRecord struct {
	Currency   string        `json:"currency"`
	InstStatus string        `json:"instStatus"`
	Chains     []chainRecord `json:"chains"`
}

type chainRecord struct {
	Chain               string `json:"chain"`
	DisplayName         string `json:"displayName"`
	FullName            string `json:"fullName"`
	BaseChain           string `json:"baseChain"`
	BaseChainProtocol   string `json:"baseChainProtocol"`
	DepositStatus       string `json:"depositStatus"`
	MinDepositAmount    string `json:"minDepositAmt"`
	WithdrawStatus      string `json:"withdrawStatus"`
	MinWithdrawAmount   string `json:"minWithdrawAmt"`
	TransactFeeWithdraw string `json:"transactFeeWithdraw"`
	WithdrawFeeType     string `json:"withdrawFeeType"`
	NumConfirmations    *int   `json:"numOfConfirmations"`
}
