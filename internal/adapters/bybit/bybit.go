package bybit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	Slug           = "bybit"
	defaultBaseURL = "https://api.bybit.com"

	coinInfoPath = "/v5/asset/coin/query-info"

	defaultRecvWindow = "10000"
	maxResponseBytes  = 10 << 20
)

var _ types.Adapter = (*Adapter)(nil)

type Adapter struct {
	baseURL    string
	client     *http.Client
	apiKey     string
	apiSecret  string
	recvWindow string
	referer    string
	now        func() time.Time
}

type Option func(*Adapter)

func New(opts ...Option) *Adapter {
	adapter := &Adapter{
		baseURL:    defaultBaseURL,
		client:     http.DefaultClient,
		recvWindow: defaultRecvWindow,
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(adapter)
	}
	if adapter.client == nil {
		adapter.client = http.DefaultClient
	}
	if adapter.recvWindow == "" {
		adapter.recvWindow = defaultRecvWindow
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

func WithCredentials(apiKey string, apiSecret string) Option {
	return func(adapter *Adapter) {
		adapter.apiKey = strings.TrimSpace(apiKey)
		adapter.apiSecret = strings.TrimSpace(apiSecret)
	}
}

func WithRecvWindow(recvWindow string) Option {
	return func(adapter *Adapter) {
		adapter.recvWindow = strings.TrimSpace(recvWindow)
	}
}

func WithReferer(referer string) Option {
	return func(adapter *Adapter) {
		adapter.referer = strings.TrimSpace(referer)
	}
}

func WithNow(now func() time.Time) Option {
	return func(adapter *Adapter) {
		adapter.now = now
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

	var payload coinInfoResponse
	if err := adapter.getJSON(ctx, coinInfoPath, "", &payload); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch bybit coin info: %w", err)
	}
	if payload.RetCode != 0 {
		return types.FetchResult{}, fmt.Errorf("fetch bybit coin info: retCode %d: %s", payload.RetCode, strings.TrimSpace(payload.RetMsg))
	}

	now := adapter.now
	if now == nil {
		now = time.Now
	}
	return snapshotsFromCoinInfo(payload, now().UTC())
}

func (a *Adapter) getJSON(ctx context.Context, path string, rawQuery string, dest any) error {
	if a.baseURL == "" {
		a.baseURL = defaultBaseURL
	}
	if a.client == nil {
		a.client = http.DefaultClient
	}

	req, err := a.newSignedGETRequest(ctx, path, rawQuery)
	if err != nil {
		return err
	}

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

func (a *Adapter) newSignedGETRequest(ctx context.Context, path string, rawQuery string) (*http.Request, error) {
	if strings.TrimSpace(a.apiKey) == "" {
		return nil, fmt.Errorf("missing bybit api key")
	}
	if strings.TrimSpace(a.apiSecret) == "" {
		return nil, fmt.Errorf("missing bybit api secret")
	}

	baseURL := strings.TrimRight(a.baseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	url := baseURL + path
	if rawQuery != "" {
		url += "?" + rawQuery
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	now := a.now
	if now == nil {
		now = time.Now
	}
	recvWindow := strings.TrimSpace(a.recvWindow)
	if recvWindow == "" {
		recvWindow = defaultRecvWindow
	}
	timestamp := strconv.FormatInt(now().UTC().UnixMilli(), 10)
	signature := signHMACSHA256(a.apiSecret, timestamp+a.apiKey+recvWindow+rawQuery)

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cex-router/1.0")
	req.Header.Set("X-BAPI-API-KEY", a.apiKey)
	req.Header.Set("X-BAPI-SIGN", signature)
	req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
	req.Header.Set("X-BAPI-RECV-WINDOW", recvWindow)
	if referer := strings.TrimSpace(a.referer); referer != "" {
		req.Header.Set("X-Referer", referer)
		req.Header.Set("Referer", referer)
	}

	return req, nil
}

func signHMACSHA256(secret string, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func snapshotsFromCoinInfo(payload coinInfoResponse, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	for _, row := range payload.Result.Rows {
		symbol := strings.ToUpper(strings.TrimSpace(row.Coin))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped asset with empty coin")
			continue
		}
		if len(row.Chains) == 0 {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no chains", symbol))
			continue
		}

		for _, chain := range row.Chains {
			rawChain := strings.TrimSpace(chain.Chain)
			rawChainName := strings.TrimSpace(chain.ChainType)
			if rawChain == "" && rawChainName == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s chain with empty chain identifiers", symbol))
				continue
			}

			snapshot, notes, err := snapshotFromChain(row, symbol, chain, observedAt)
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
		return types.FetchResult{}, fmt.Errorf("bybit response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromChain(row coinInfoRow, symbol string, chain coinInfoChain, observedAt time.Time) (types.RailSnapshot, []string, error) {
	withdrawMin, _, err := optionalDecimal(chain.WithdrawMin)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s withdraw_min: %w", symbol, chainLabel(chain), err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(chain.WithdrawFee)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s withdraw_fee: %w", symbol, chainLabel(chain), err)
	}
	withdrawPercentageFee, hasWithdrawPercentageFee, err := optionalNonZeroDecimal(chain.WithdrawPercentageFee)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s withdraw_percentage_fee: %w", symbol, chainLabel(chain), err)
	}
	depositConfirmations, err := optionalInt(chain.Confirmation)
	if err != nil {
		return types.RailSnapshot{}, nil, fmt.Errorf("%s %s confirmation: %w", symbol, chainLabel(chain), err)
	}

	depositEnabled, depositOK := parseBybitStatus(chain.ChainDeposit)
	withdrawEnabled, withdrawOK := parseBybitStatus(chain.ChainWithdraw)
	if !hasWithdrawFee {
		withdrawEnabled = false
	}

	var notes []string
	if !depositOK {
		notes = append(notes, fmt.Sprintf("unexpected deposit status for %s %s: %q", symbol, chainLabel(chain), chain.ChainDeposit))
	}
	if !withdrawOK {
		notes = append(notes, fmt.Sprintf("unexpected withdraw status for %s %s: %q", symbol, chainLabel(chain), chain.ChainWithdraw))
	}

	snapshot := types.RailSnapshot{
		ExchangeSlug:         Slug,
		CoinSymbol:           symbol,
		CoinName:             strings.TrimSpace(row.Name),
		RawChainSymbol:       strings.TrimSpace(chain.Chain),
		RawChainName:         strings.TrimSpace(chain.ChainType),
		RawNetworkID:         strings.TrimSpace(chain.Chain),
		ContractAddress:      strings.TrimSpace(chain.ContractAddress),
		DepositEnabled:       depositEnabled,
		WithdrawEnabled:      withdrawEnabled,
		DepositConfirmations: depositConfirmations,
		WithdrawMin:          withdrawMin,
		WithdrawFee:          withdrawFee,
		ObservedAt:           observedAt,
	}

	switch {
	case hasWithdrawFee && hasWithdrawPercentageFee:
		snapshot.WithdrawFeeType = types.FeeTypeHybrid
		snapshot.WithdrawFeePercent = withdrawPercentageFee
	case hasWithdrawFee:
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	case hasWithdrawPercentageFee:
		snapshot.WithdrawFeeType = types.FeeTypePercent
		snapshot.WithdrawFeePercent = withdrawPercentageFee
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
	return parsed, true, nil
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

func parseBybitStatus(raw string) (bool, bool) {
	switch strings.TrimSpace(raw) {
	case "0":
		return false, true
	case "1":
		return true, true
	default:
		return false, false
	}
}

func chainLabel(chain coinInfoChain) string {
	if value := strings.TrimSpace(chain.Chain); value != "" {
		return value
	}
	return strings.TrimSpace(chain.ChainType)
}

type coinInfoResponse struct {
	RetCode int            `json:"retCode"`
	RetMsg  string         `json:"retMsg"`
	Result  coinInfoResult `json:"result"`
}

type coinInfoResult struct {
	Rows []coinInfoRow `json:"rows"`
}

type coinInfoRow struct {
	Name   string          `json:"name"`
	Coin   string          `json:"coin"`
	Chains []coinInfoChain `json:"chains"`
}

type coinInfoChain struct {
	Chain                 string `json:"chain"`
	ChainType             string `json:"chainType"`
	Confirmation          string `json:"confirmation"`
	WithdrawFee           string `json:"withdrawFee"`
	DepositMin            string `json:"depositMin"`
	WithdrawMin           string `json:"withdrawMin"`
	MinAccuracy           string `json:"minAccuracy"`
	ChainDeposit          string `json:"chainDeposit"`
	ChainWithdraw         string `json:"chainWithdraw"`
	WithdrawPercentageFee string `json:"withdrawPercentageFee"`
	ContractAddress       string `json:"contractAddress"`
	SafeConfirmNumber     string `json:"safeConfirmNumber"`
	WithdrawMax           string `json:"withdrawMax"`
}
