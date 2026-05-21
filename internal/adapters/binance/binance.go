package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

const (
	Slug           = "binance"
	defaultBaseURL = "https://api.binance.com"

	capitalConfigPath = "/sapi/v1/capital/config/getall"
	getAllWeight      = 10

	defaultRecvWindowMillis = int64(10_000)
	defaultWeightPerMinute  = 1200
	defaultWeightBurst      = 1200

	maxResponseBytes = 20 << 20
)

var _ types.Adapter = (*Adapter)(nil)

type Limiter interface {
	WaitN(ctx context.Context, n int) error
}

type Adapter struct {
	baseURL          string
	apiKey           string
	apiSecret        string
	recvWindowMillis int64
	client           *http.Client
	limiter          Limiter
	now              func() time.Time
}

type Option func(*Adapter)

func New(apiKey string, apiSecret string, opts ...Option) *Adapter {
	adapter := &Adapter{
		baseURL:          defaultBaseURL,
		apiKey:           apiKey,
		apiSecret:        apiSecret,
		recvWindowMillis: defaultRecvWindowMillis,
		client:           http.DefaultClient,
		limiter:          newWeightedLimiter(defaultWeightPerMinute, defaultWeightBurst),
		now:              time.Now,
	}
	for _, opt := range opts {
		opt(adapter)
	}
	adapter.ensureDefaults()
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
		adapter.apiKey = apiKey
		adapter.apiSecret = apiSecret
	}
}

func WithRecvWindow(window time.Duration) Option {
	return func(adapter *Adapter) {
		adapter.recvWindowMillis = window.Milliseconds()
	}
}

func WithLimiter(limiter Limiter) Option {
	return func(adapter *Adapter) {
		adapter.limiter = limiter
	}
}

func WithClock(now func() time.Time) Option {
	return func(adapter *Adapter) {
		adapter.now = now
	}
}

func NewWeightedLimiter(weightPerMinute int, burst int) Limiter {
	return newWeightedLimiter(weightPerMinute, burst)
}

func (a *Adapter) Slug() string {
	return Slug
}

func (a *Adapter) FetchRails(ctx context.Context) (types.FetchResult, error) {
	adapter := a
	if adapter == nil {
		adapter = New("", "")
	}
	adapter.ensureDefaults()

	if strings.TrimSpace(adapter.apiKey) == "" || strings.TrimSpace(adapter.apiSecret) == "" {
		return types.FetchResult{}, fmt.Errorf("binance credentials are required")
	}

	var assets []capitalAsset
	if err := adapter.getSignedJSON(ctx, capitalConfigPath, getAllWeight, &assets); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch binance capital config: %w", err)
	}
	if len(assets) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch binance capital config: empty response")
	}

	return snapshotsFromAssets(assets, adapter.now().UTC())
}

func (a *Adapter) getSignedJSON(ctx context.Context, path string, weight int, dest any) error {
	if a.limiter != nil {
		if err := a.limiter.WaitN(ctx, weight); err != nil {
			return err
		}
	}

	query := a.signedQuery()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+path+"?"+query, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "cex-router/1.0")
	req.Header.Set("X-MBX-APIKEY", a.apiKey)

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

func (a *Adapter) signedQuery() string {
	query := fmt.Sprintf("timestamp=%d&recvWindow=%d", a.now().UTC().UnixMilli(), a.recvWindowMillis)
	return query + "&signature=" + signQuery(query, a.apiSecret)
}

func signQuery(query string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(query))
	return hex.EncodeToString(mac.Sum(nil))
}

func snapshotsFromAssets(assets []capitalAsset, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	for _, asset := range assets {
		symbol := strings.ToUpper(strings.TrimSpace(asset.Coin))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped asset with empty coin")
			continue
		}
		if len(asset.NetworkList) == 0 {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no networks", symbol))
			continue
		}

		for _, network := range asset.NetworkList {
			rawNetworkID := strings.TrimSpace(network.Network)
			if rawNetworkID == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s network with empty network id", symbol))
				continue
			}

			depositEnabled, hasDepositEnabled := optionalBool(network.DepositEnable)
			withdrawEnabled, hasWithdrawEnabled := optionalBool(network.WithdrawEnable)
			if !hasDepositEnabled || !hasWithdrawEnabled {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("missing status field for %s %s", symbol, rawNetworkID))
			}

			withdrawMin, _, err := optionalDecimal(network.WithdrawMin)
			if err != nil {
				return types.FetchResult{}, fmt.Errorf("%s %s withdraw_min: %w", symbol, rawNetworkID, err)
			}
			withdrawFee, hasWithdrawFee, err := optionalDecimal(network.WithdrawFee)
			if err != nil {
				return types.FetchResult{}, fmt.Errorf("%s %s withdraw_fee: %w", symbol, rawNetworkID, err)
			}

			rawChainName := strings.TrimSpace(network.Name)
			if rawChainName == "" {
				rawChainName = rawNetworkID
			}

			snapshot := types.RailSnapshot{
				ExchangeSlug:         Slug,
				CoinSymbol:           symbol,
				CoinName:             strings.TrimSpace(asset.Name),
				RawChainSymbol:       rawNetworkID,
				RawChainName:         rawChainName,
				RawNetworkID:         rawNetworkID,
				DepositEnabled:       enabledByAsset(asset.DepositAllEnable, depositEnabled),
				WithdrawEnabled:      enabledByAsset(asset.WithdrawAllEnable, withdrawEnabled),
				DepositConfirmations: copyIntPtr(network.MinConfirm),
				WithdrawMin:          withdrawMin,
				WithdrawFee:          withdrawFee,
				ObservedAt:           observedAt,
			}
			if hasWithdrawFee {
				snapshot.WithdrawFeeType = types.FeeTypeFixed
			}

			result.Snapshots = append(result.Snapshots, snapshot)
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("binance response produced no rail snapshots")
	}
	return result, nil
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

func optionalBool(raw *bool) (bool, bool) {
	if raw == nil {
		return false, false
	}
	return *raw, true
}

func enabledByAsset(assetEnabled *bool, networkEnabled bool) bool {
	if assetEnabled == nil {
		return networkEnabled
	}
	return *assetEnabled && networkEnabled
}

func copyIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (a *Adapter) ensureDefaults() {
	if strings.TrimSpace(a.baseURL) == "" {
		a.baseURL = defaultBaseURL
	}
	a.baseURL = strings.TrimRight(a.baseURL, "/")
	if a.client == nil {
		a.client = http.DefaultClient
	}
	if a.recvWindowMillis <= 0 {
		a.recvWindowMillis = defaultRecvWindowMillis
	}
	if a.now == nil {
		a.now = time.Now
	}
}

type weightedLimiter struct {
	mu              sync.Mutex
	capacity        float64
	tokens          float64
	refillPerSecond float64
	last            time.Time
}

func newWeightedLimiter(weightPerMinute int, burst int) Limiter {
	if weightPerMinute <= 0 || burst <= 0 {
		return noopLimiter{}
	}
	return &weightedLimiter{
		capacity:        float64(burst),
		tokens:          float64(burst),
		refillPerSecond: float64(weightPerMinute) / 60,
		last:            time.Now(),
	}
}

func (l *weightedLimiter) WaitN(ctx context.Context, n int) error {
	if l == nil || n <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if float64(n) > l.capacity {
		return fmt.Errorf("rate limiter weight %d exceeds burst %.0f", n, l.capacity)
	}

	for {
		wait, ok := l.reserve(n, time.Now())
		if ok {
			return nil
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *weightedLimiter) reserve(n int, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.last.IsZero() {
		l.last = now
	}
	if elapsed := now.Sub(l.last); elapsed > 0 {
		l.tokens += elapsed.Seconds() * l.refillPerSecond
		if l.tokens > l.capacity {
			l.tokens = l.capacity
		}
		l.last = now
	}

	needed := float64(n)
	if l.tokens >= needed {
		l.tokens -= needed
		return 0, true
	}

	missing := needed - l.tokens
	wait := time.Duration((missing / l.refillPerSecond) * float64(time.Second))
	if wait < time.Millisecond {
		wait = time.Millisecond
	}
	return wait, false
}

type noopLimiter struct{}

func (noopLimiter) WaitN(context.Context, int) error {
	return nil
}

type capitalAsset struct {
	Coin              string           `json:"coin"`
	Name              string           `json:"name"`
	DepositAllEnable  *bool            `json:"depositAllEnable"`
	WithdrawAllEnable *bool            `json:"withdrawAllEnable"`
	NetworkList       []capitalNetwork `json:"networkList"`
}

type capitalNetwork struct {
	Coin           string `json:"coin"`
	Name           string `json:"name"`
	Network        string `json:"network"`
	DepositEnable  *bool  `json:"depositEnable"`
	WithdrawEnable *bool  `json:"withdrawEnable"`
	MinConfirm     *int   `json:"minConfirm"`
	WithdrawFee    string `json:"withdrawFee"`
	WithdrawMin    string `json:"withdrawMin"`
}
