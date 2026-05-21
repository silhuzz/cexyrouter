package gate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/silhuzz/cexyrouter/pkg/types"
)

const (
	Slug           = "gate"
	defaultBaseURL = "https://api.gateio.ws"

	currenciesPath = "/api/v4/spot/currencies"

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

	var currencies []currencyRecord
	if err := adapter.getJSON(ctx, currenciesPath, &currencies); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch gate currencies: %w", err)
	}
	if len(currencies) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch gate currencies: empty data")
	}

	return snapshotsFromCurrencies(currencies, adapter.now().UTC())
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
		symbol := coinSymbol(currency.Currency)
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped currency with empty symbol")
			continue
		}

		if len(currency.Chains) == 0 {
			rawChain := firstNonEmpty(strings.TrimSpace(currency.Chain), strings.TrimSpace(currency.Currency))
			if rawChain == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with empty chain", symbol))
				continue
			}
			result.Snapshots = append(result.Snapshots, snapshotFromStatus(currency, symbol, rawChain, rawChain, currency.DepositDisabled, currency.WithdrawDisabled, currency.WithdrawDelayed, currency.Delisted, observedAt))
			continue
		}

		for _, chain := range currency.Chains {
			rawChain := strings.TrimSpace(chain.Name)
			if rawChain == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s chain with empty name", symbol))
				continue
			}
			result.Snapshots = append(result.Snapshots, snapshotFromStatus(currency, symbol, rawChain, rawChain, chain.DepositDisabled, chain.WithdrawDisabled, chain.WithdrawDelayed, currency.Delisted, observedAt))
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("gate response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromStatus(currency currencyRecord, symbol string, rawChain string, rawNetworkID string, depositDisabled bool, withdrawDisabled bool, withdrawDelayed bool, delisted bool, observedAt time.Time) types.RailSnapshot {
	return types.RailSnapshot{
		ExchangeSlug:    Slug,
		CoinSymbol:      symbol,
		CoinName:        firstNonEmpty(strings.TrimSpace(currency.Name), symbol),
		RawChainSymbol:  rawChain,
		RawChainName:    rawChain,
		RawNetworkID:    rawNetworkID,
		DepositEnabled:  !delisted && !depositDisabled,
		WithdrawEnabled: !delisted && !withdrawDisabled && !withdrawDelayed,
		ObservedAt:      observedAt,
	}
}

func coinSymbol(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	if before, _, ok := strings.Cut(raw, "_"); ok && before != "" {
		return before
	}
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type currencyRecord struct {
	Currency         string        `json:"currency"`
	Name             string        `json:"name"`
	Delisted         bool          `json:"delisted"`
	WithdrawDisabled bool          `json:"withdraw_disabled"`
	WithdrawDelayed  bool          `json:"withdraw_delayed"`
	DepositDisabled  bool          `json:"deposit_disabled"`
	TradeDisabled    bool          `json:"trade_disabled"`
	FixedRate        string        `json:"fixed_rate"`
	Chain            string        `json:"chain"`
	Chains           []chainRecord `json:"chains"`
}

type chainRecord struct {
	Name             string `json:"name"`
	Addr             string `json:"addr"`
	WithdrawDisabled bool   `json:"withdraw_disabled"`
	WithdrawDelayed  bool   `json:"withdraw_delayed"`
	DepositDisabled  bool   `json:"deposit_disabled"`
}
