package bithumb

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
	Slug           = "bithumb"
	defaultBaseURL = "https://api.bithumb.com"

	feePath           = "/v2/fee/inout/ALL"
	networkStatusPath = "/public/assetsstatus/multichain/ALL"

	maxResponseBytes = 10 << 20
)

var _ types.Adapter = (*Adapter)(nil)

type Adapter struct {
	baseURL string
	client  *http.Client
}

type Option func(*Adapter)

func New(opts ...Option) *Adapter {
	adapter := &Adapter{
		baseURL: defaultBaseURL,
		client:  http.DefaultClient,
	}
	for _, opt := range opts {
		opt(adapter)
	}
	if adapter.client == nil {
		adapter.client = http.DefaultClient
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

func (a *Adapter) Slug() string {
	return Slug
}

func (a *Adapter) FetchRails(ctx context.Context) (types.FetchResult, error) {
	adapter := a
	if adapter == nil {
		adapter = New()
	}

	var fees []feeAsset
	if err := adapter.getJSON(ctx, feePath, &fees); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch bithumb fee data: %w", err)
	}
	if len(fees) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch bithumb fee data: empty response")
	}

	var statuses networkStatusResponse
	if err := adapter.getJSON(ctx, networkStatusPath, &statuses); err != nil {
		return types.FetchResult{}, fmt.Errorf("fetch bithumb network status: %w", err)
	}
	if statuses.Status != "0000" {
		return types.FetchResult{}, fmt.Errorf("fetch bithumb network status: status %q", statuses.Status)
	}
	if len(statuses.Data) == 0 {
		return types.FetchResult{}, fmt.Errorf("fetch bithumb network status: empty data")
	}

	return snapshotsFromResponses(fees, indexNetworkStatuses(statuses.Data), time.Now().UTC())
}

func (a *Adapter) getJSON(ctx context.Context, path string, dest any) error {
	if a.baseURL == "" {
		a.baseURL = defaultBaseURL
	}
	if a.client == nil {
		a.client = http.DefaultClient
	}
	url := strings.TrimRight(a.baseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

func snapshotsFromResponses(fees []feeAsset, statuses map[string][]networkStatus, observedAt time.Time) (types.FetchResult, error) {
	result := types.FetchResult{
		Snapshots: make([]types.RailSnapshot, 0),
		Complete:  true,
	}

	for _, asset := range fees {
		symbol := strings.ToUpper(strings.TrimSpace(asset.Currency))
		if symbol == "" {
			result.Complete = false
			result.Notes = append(result.Notes, "skipped asset with empty currency")
			continue
		}
		if len(asset.Networks) == 0 {
			result.Complete = false
			result.Notes = append(result.Notes, fmt.Sprintf("skipped %s with no networks", symbol))
			continue
		}

		for _, network := range asset.Networks {
			netName := strings.TrimSpace(network.Name)
			if netName == "" {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("skipped %s network with empty name", symbol))
				continue
			}

			status, hasStatus := findNetworkStatus(statuses, symbol, netName)
			if !hasStatus {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("missing network status for %s %s", symbol, netName))
			} else if !isBinaryStatus(status.DepositStatus) || !isBinaryStatus(status.WithdrawalStatus) {
				result.Complete = false
				result.Notes = append(result.Notes, fmt.Sprintf("unexpected network status value for %s %s", symbol, status.NetworkType))
			}

			snapshot, err := snapshotFromNetwork(asset, symbol, status, hasStatus, network, netName, observedAt)
			if err != nil {
				return types.FetchResult{}, err
			}
			result.Snapshots = append(result.Snapshots, snapshot)
		}
	}

	if len(result.Snapshots) == 0 {
		return types.FetchResult{}, fmt.Errorf("bithumb response produced no rail snapshots")
	}
	return result, nil
}

func snapshotFromNetwork(asset feeAsset, symbol string, status networkStatus, hasStatus bool, network feeNetwork, netName string, observedAt time.Time) (types.RailSnapshot, error) {
	withdrawMin, _, err := optionalDecimal(network.WithdrawMinimumQuantity)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_min: %w", symbol, netName, err)
	}
	withdrawFee, hasWithdrawFee, err := optionalDecimal(network.WithdrawFeeQuantity)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_fee: %w", symbol, netName, err)
	}
	withdrawRate, hasWithdrawRate, err := optionalDecimal(network.WithdrawRate)
	if err != nil {
		return types.RailSnapshot{}, fmt.Errorf("%s %s withdraw_rate: %w", symbol, netName, err)
	}

	snapshot := types.RailSnapshot{
		ExchangeSlug:    Slug,
		CoinSymbol:      symbol,
		CoinName:        strings.TrimSpace(asset.Name),
		RawChainSymbol:  netName,
		RawChainName:    netName,
		RawNetworkID:    netName,
		DepositEnabled:  hasStatus && status.DepositStatus == 1,
		WithdrawEnabled: hasStatus && status.WithdrawalStatus == 1,
		WithdrawMin:     withdrawMin,
		WithdrawFee:     withdrawFee,
		ObservedAt:      observedAt,
	}
	if hasStatus {
		snapshot.RawChainSymbol = strings.TrimSpace(status.NetworkType)
		snapshot.RawNetworkID = strings.TrimSpace(status.NetworkType)
	}

	switch {
	case hasWithdrawFee && hasWithdrawRate:
		snapshot.WithdrawFeeType = types.FeeTypeHybrid
		snapshot.WithdrawFeePercent = withdrawRate
	case hasWithdrawFee:
		snapshot.WithdrawFeeType = types.FeeTypeFixed
	case hasWithdrawRate:
		snapshot.WithdrawFeeType = types.FeeTypePercent
		snapshot.WithdrawFeePercent = withdrawRate
	}

	return snapshot, nil
}

func optionalDecimal(raw *string) (*decimal.Decimal, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, false, nil
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return nil, false, err
	}
	return &parsed, true, nil
}

func isBinaryStatus(value int) bool {
	return value == 0 || value == 1
}

func indexNetworkStatuses(statuses []networkStatus) map[string][]networkStatus {
	index := make(map[string][]networkStatus)
	for _, status := range statuses {
		symbol := strings.ToUpper(strings.TrimSpace(status.Currency))
		if symbol == "" {
			continue
		}
		index[symbol] = append(index[symbol], status)
	}
	return index
}

func findNetworkStatus(index map[string][]networkStatus, symbol string, netName string) (networkStatus, bool) {
	statuses := index[symbol]
	if len(statuses) == 0 {
		return networkStatus{}, false
	}

	candidates := networkCandidates(netName)
	for _, status := range statuses {
		if _, ok := candidates[normalizeNetworkCode(status.NetworkType)]; ok {
			return status, true
		}
	}

	if len(statuses) == 1 {
		return statuses[0], true
	}
	return networkStatus{}, false
}

func networkCandidates(netName string) map[string]struct{} {
	candidates := make(map[string]struct{})
	normalized := normalizeNetworkCode(netName)
	if normalized == "" {
		return candidates
	}
	candidates[normalized] = struct{}{}
	for _, alias := range networkAliases[normalized] {
		candidates[normalizeNetworkCode(alias)] = struct{}{}
	}
	return candidates
}

func normalizeNetworkCode(raw string) string {
	var builder strings.Builder
	for _, char := range strings.ToUpper(strings.TrimSpace(raw)) {
		if (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

var networkAliases = map[string][]string{
	"APTOS":           {"APT"},
	"ARBITRUMONE":     {"ARB_ETH", "ARB"},
	"AVALANCHE":       {"AVAX"},
	"BASE":            {"BASE_ETH"},
	"BLAST":           {"BLAST_ETH"},
	"BITCOIN":         {"BTC"},
	"BITCOINCASH":     {"BCH"},
	"BITCOINSV":       {"BSV"},
	"BNBSMARTCHAIN":   {"BSC"},
	"CARDANO":         {"ADA"},
	"ETHEREUM":        {"ETH"},
	"ETHEREUMCLASSIC": {"ETC"},
	"KAIA":            {"KAIA"},
	"LINEA":           {"LINEA"},
	"MANTA":           {"MANTA_ETH"},
	"MEGAETH":         {"MEGA_ETH"},
	"OPTIMISM":        {"OP_ETH", "OP"},
	"POLYGON":         {"POL", "MATIC"},
	"SCROLL":          {"SCROLL_ETH"},
	"STABLE":          {"STABLE_USDT"},
	"SOLANA":          {"SOL"},
	"STELLAR":         {"XLM"},
	"STELLARLUMENS":   {"XLM"},
	"STARKNET":        {"STRK"},
	"SWELLCHAIN":      {"SWELL_ETH"},
	"TAIKO":           {"TAIKO_ETH"},
	"THETANETWORK":    {"THETA"},
	"TRON":            {"TRX"},
	"WORLDCHAIN":      {"WLD_ETH"},
	"ZETACHAIN":       {"ZETA"},
	"ZKSYNCERA":       {"ZK_ETH"},
	"XRPLEDGER":       {"XRP"},
}

type feeAsset struct {
	Name     string       `json:"name"`
	Currency string       `json:"currency"`
	Networks []feeNetwork `json:"networks"`
}

type feeNetwork struct {
	Name                    string  `json:"net_name"`
	WithdrawFeeQuantity     *string `json:"withdraw_fee_quantity"`
	WithdrawMinimumQuantity *string `json:"withdraw_minimum_quantity"`
	WithdrawRate            *string `json:"withdraw_rate"`
}

type networkStatusResponse struct {
	Status string          `json:"status"`
	Data   []networkStatus `json:"data"`
}

type networkStatus struct {
	Currency         string `json:"currency"`
	NetworkType      string `json:"net_type"`
	WithdrawalStatus int    `json:"withdrawal_status"`
	DepositStatus    int    `json:"deposit_status"`
}
