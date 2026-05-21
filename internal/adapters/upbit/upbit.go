package upbit

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/silhuzz/cexyrouter/pkg/types"
)

const (
	Slug           = "upbit"
	defaultBaseURL = "https://api.upbit.com"

	walletStatusPath = "/v1/status/wallet"

	maxResponseBytes = 10 << 20
)

var _ types.Adapter = (*Adapter)(nil)

type Adapter struct {
	baseURL     string
	accessKey   string
	secretKey   string
	client      *http.Client
	nonceSource func() (string, error)
}

type Option func(*Adapter)

func New(opts ...Option) *Adapter {
	adapter := &Adapter{
		baseURL:     defaultBaseURL,
		client:      http.DefaultClient,
		nonceSource: newUUIDNonce,
	}
	for _, opt := range opts {
		opt(adapter)
	}
	if adapter.client == nil {
		adapter.client = http.DefaultClient
	}
	if adapter.nonceSource == nil {
		adapter.nonceSource = newUUIDNonce
	}
	adapter.baseURL = strings.TrimRight(adapter.baseURL, "/")
	return adapter
}

func WithBaseURL(baseURL string) Option {
	return func(adapter *Adapter) {
		adapter.baseURL = baseURL
	}
}

func WithCredentials(accessKey, secretKey string) Option {
	return func(adapter *Adapter) {
		adapter.accessKey = accessKey
		adapter.secretKey = secretKey
	}
}

func WithHTTPClient(client *http.Client) Option {
	return func(adapter *Adapter) {
		adapter.client = client
	}
}

func WithNonceSource(nonceSource func() (string, error)) Option {
	return func(adapter *Adapter) {
		adapter.nonceSource = nonceSource
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

	var statuses []walletStatus
	if err := adapter.getJSON(ctx, walletStatusPath, "", &statuses); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch upbit wallet status: %w", err)
	}
	if len(statuses) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch upbit wallet status: empty response")
	}

	return snapshotsFromWalletStatuses(statuses, time.Now().UTC())
}

func (a *Adapter) getJSON(ctx context.Context, path string, literalQuery string, dest any) error {
	if a.baseURL == "" {
		a.baseURL = defaultBaseURL
	}
	if a.client == nil {
		a.client = http.DefaultClient
	}

	token, err := a.authToken(literalQuery)
	if err != nil {
		return err
	}

	url := strings.TrimRight(a.baseURL, "/") + path
	if literalQuery != "" {
		url += "?" + literalQuery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
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

func (a *Adapter) authToken(literalQuery string) (string, error) {
	if a.accessKey == "" || a.secretKey == "" {
		return "", fmt.Errorf("upbit access key and secret key are required")
	}
	if a.nonceSource == nil {
		a.nonceSource = newUUIDNonce
	}
	nonce, err := a.nonceSource()
	if err != nil {
		return "", fmt.Errorf("create upbit nonce: %w", err)
	}
	return newJWT(a.accessKey, a.secretKey, nonce, literalQuery)
}

func newJWT(accessKey, secretKey, nonce, literalQuery string) (string, error) {
	headerJSON, err := json.Marshal(jwtHeader{
		Algorithm: "HS256",
		Type:      "JWT",
	})
	if err != nil {
		return "", err
	}

	payload := jwtPayload{
		AccessKey: accessKey,
		Nonce:     nonce,
	}
	if literalQuery != "" {
		payload.QueryHash = sha512Hex(literalQuery)
		payload.QueryHashAlg = "SHA512"
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	unsigned := base64URL(headerJSON) + "." + base64URL(payloadJSON)
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64URL(mac.Sum(nil)), nil
}

func snapshotsFromWalletStatuses(statuses []walletStatus, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0, len(statuses)),
		Complete:  true,
	}

	for _, status := range statuses {
		symbol := strings.ToUpper(strings.TrimSpace(status.Currency))
		netType := strings.TrimSpace(status.NetType)
		networkName := strings.TrimSpace(status.NetworkName)
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped wallet status with empty currency")
			continue
		}
		if netType == "" {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s wallet status with empty net_type", symbol))
			continue
		}

		depositEnabled, withdrawEnabled, ok := walletAvailability(status.WalletState)
		if !ok {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("unexpected wallet_state for %s %s: %q", symbol, netType, status.WalletState))
		}

		result.Snapshots = append(result.Snapshots, types.RailSnapshot{
			ExchangeSlug:    Slug,
			CoinSymbol:      symbol,
			RawChainSymbol:  netType,
			RawChainName:    networkName,
			RawNetworkID:    netType,
			DepositEnabled:  depositEnabled,
			WithdrawEnabled: withdrawEnabled,
			ObservedAt:      observedAt,
		})
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("upbit wallet status produced no rail snapshots")
	}
	return result, nil
}

func walletAvailability(walletState string) (depositEnabled bool, withdrawEnabled bool, ok bool) {
	switch strings.ToLower(strings.TrimSpace(walletState)) {
	case "working":
		return true, true, true
	case "withdraw_only":
		return false, true, true
	case "deposit_only":
		return true, false, true
	case "paused", "unsupported":
		return false, false, true
	default:
		return false, false, false
	}
}

func newUUIDNonce() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return strings.Join([]string{
		hex.EncodeToString(bytes[0:4]),
		hex.EncodeToString(bytes[4:6]),
		hex.EncodeToString(bytes[6:8]),
		hex.EncodeToString(bytes[8:10]),
		hex.EncodeToString(bytes[10:16]),
	}, "-"), nil
}

func sha512Hex(value string) string {
	sum := sha512.Sum512([]byte(value))
	return hex.EncodeToString(sum[:])
}

func base64URL(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type jwtPayload struct {
	AccessKey    string `json:"access_key"`
	Nonce        string `json:"nonce"`
	QueryHash    string `json:"query_hash,omitempty"`
	QueryHashAlg string `json:"query_hash_alg,omitempty"`
}

type walletStatus struct {
	Currency            string  `json:"currency"`
	WalletState         string  `json:"wallet_state"`
	BlockState          *string `json:"block_state"`
	BlockHeight         *int64  `json:"block_height"`
	BlockUpdatedAt      *string `json:"block_updated_at"`
	BlockElapsedMinutes *int    `json:"block_elapsed_minutes"`
	NetType             string  `json:"net_type"`
	NetworkName         string  `json:"network_name"`
}
