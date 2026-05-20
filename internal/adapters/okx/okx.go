package okx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pedro/cex-router/pkg/types"
	"github.com/shopspring/decimal"
)

const (
	Slug           = "okx"
	defaultBaseURL = "https://eea.okx.com"

	currenciesPath = "/api/v5/asset/currencies"

	maxResponseBytes = 10 << 20
)

var _ types.Adapter = (*Adapter)(nil)

type Adapter struct {
	baseURL    string
	apiKey     string
	apiSecret  string
	passphrase string
	simulated  bool
	client     *http.Client
	now        func() time.Time
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

func WithCredentials(apiKey string, apiSecret string, passphrase string) Option {
	return func(adapter *Adapter) {
		adapter.apiKey = strings.TrimSpace(apiKey)
		adapter.apiSecret = strings.TrimSpace(apiSecret)
		adapter.passphrase = strings.TrimSpace(passphrase)
	}
}

func WithSimulatedTrading(enabled bool) Option {
	return func(adapter *Adapter) {
		adapter.simulated = enabled
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
	if strings.TrimSpace(adapter.apiKey) == "" || strings.TrimSpace(adapter.apiSecret) == "" || strings.TrimSpace(adapter.passphrase) == "" {
		return types.FetchResult{}, fmt.Errorf("okx credentials are required")
	}

	var response currenciesResponse
	if err := adapter.getJSON(ctx, currenciesPath, &response); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch okx currencies: %w", err)
	}
	if response.Code != "0" {
		return types.FetchResult{}, fmt.Errorf("fetch okx currencies: code %q msg %q", response.Code, response.Message)
	}
	if len(response.Data) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch okx currencies: empty data")
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
	if a.now == nil {
		a.now = time.Now
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(a.baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	a.signRequest(req, nil, a.now().UTC())

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

func (a *Adapter) signRequest(req *http.Request, body []byte, timestamp time.Time) {
	stamp := formatTimestamp(timestamp)
	method := strings.ToUpper(req.Method)
	requestPath := req.URL.RequestURI()
	signature := sign(a.apiSecret, stamp, method, requestPath, body)

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cex-router/1.0")
	req.Header.Set("OK-ACCESS-KEY", a.apiKey)
	req.Header.Set("OK-ACCESS-SIGN", signature)
	req.Header.Set("OK-ACCESS-TIMESTAMP", stamp)
	req.Header.Set("OK-ACCESS-PASSPHRASE", a.passphrase)
	if a.simulated {
		req.Header.Set("x-simulated-trading", "1")
	}
}

func formatTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func sign(secret string, timestamp string, method string, requestPath string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte(strings.ToUpper(method)))
	mac.Write([]byte(requestPath))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func snapshotsFromCurrencies(records []currencyRecord, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0, len(records)),
		Complete:  true,
	}

	for _, record := range records {
		symbol := strings.ToUpper(strings.TrimSpace(record.Currency))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped currency with empty ccy")
			continue
		}

		chain := strings.TrimSpace(record.Chain)
		if chain == "" {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with empty chain", symbol))
			continue
		}

		snapshot, err := snapshotFromCurrency(record, symbol, chain, observedAt)
		if err != nil {
			return types.FetchResult{}, err
		}
		result.Snapshots = append(result.Snapshots, snapshot)
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("okx response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromCurrency(record currencyRecord, symbol string, chain string, observedAt time.Time) (types.RailSnapshot, error) {
	withdrawMin, _, err := optionalDecimal(record.MinWithdraw)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s min_wd: %w", symbol, chain, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(record.MinFee)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s min_fee: %w", symbol, chain, err)
	}
	depositConfirmations, err := optionalInt(record.MinDepositArrivalConfirm)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s min_dep_arrival_confirm: %w", symbol, chain, err)
	}

	network := chainNetwork(symbol, chain)
	snapshot := types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             strings.TrimSpace(record.Name),
		RawChainSymbol:       network,
		RawChainName:         network,
		RawNetworkID:         chain,
		ContractAddress:      strings.TrimSpace(record.ContractAddress),
		DepositEnabled:       record.CanDeposit.Bool(),
		WithdrawEnabled:      record.CanWithdraw.Bool(),
		DepositConfirmations: depositConfirmations,
		WithdrawMin:          withdrawMin,
		WithdrawFee:          withdrawFee,
		ObservedAt:           observedAt,
	}
	if hasWithdrawFee {
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	}
	return snapshot, nil
}

func chainNetwork(symbol string, chain string) string {
	prefix := symbol + "-"
	if strings.HasPrefix(strings.ToUpper(chain), prefix) && len(chain) > len(prefix) {
		return strings.TrimSpace(chain[len(prefix):])
	}
	return chain
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

type currenciesResponse struct {
	Code    string           `json:"code"`
	Message string           `json:"msg"`
	Data    []currencyRecord `json:"data"`
}

type currencyRecord struct {
	Currency                 string   `json:"ccy"`
	Name                     string   `json:"name"`
	Chain                    string   `json:"chain"`
	CanDeposit               flexBool `json:"canDep"`
	CanWithdraw              flexBool `json:"canWd"`
	ContractAddress          string   `json:"ctAddr"`
	MinWithdraw              string   `json:"minWd"`
	MinFee                   string   `json:"minFee"`
	MinDepositArrivalConfirm string   `json:"minDepArrivalConfirm"`
}

type flexBool bool

func (b *flexBool) UnmarshalJSON(data []byte) error {
	value := strings.TrimSpace(string(data))
	switch value {
	case "true", `"true"`, `"1"`, "1":
		*b = true
		return nil
	case "false", `"false"`, `"0"`, "0", "null", `""`:
		*b = false
		return nil
	default:
		return fmt.Errorf("unsupported bool value %s", value)
	}
}

func (b flexBool) Bool() bool {
	return bool(b)
}
