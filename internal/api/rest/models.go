package rest

import (
	"encoding/json"
	"time"

	"github.com/pedro/cex-router/internal/api/eventmeta"
	"github.com/shopspring/decimal"
)

type exchangeRef struct {
	ID     int32  `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Region string `json:"region"`
}

type exchangeListItem = exchangeRef

type coinRef struct {
	ID     int32  `json:"id"`
	Slug   string `json:"slug"`
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

type coinListItem struct {
	coinRef
	ExternalIDs json.RawMessage `json:"external_ids"`
}

type chainRef struct {
	ID            int32  `json:"id"`
	Slug          string `json:"slug"`
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	EVMChainID    *int32 `json:"evm_chain_id"`
	ParentChainID *int32 `json:"parent_chain_id"`
}

type rail struct {
	ID                   int64       `json:"id"`
	Exchange             exchangeRef `json:"exchange"`
	Coin                 coinRef     `json:"coin"`
	Chain                chainRef    `json:"chain"`
	DepositEnabled       bool        `json:"deposit_enabled"`
	WithdrawEnabled      bool        `json:"withdraw_enabled"`
	DepositConfirmations *int32      `json:"deposit_confirmations"`
	WithdrawMin          *string     `json:"withdraw_min"`
	WithdrawFee          *string     `json:"withdraw_fee"`
	WithdrawFeeType      *string     `json:"withdraw_fee_type"`
	WithdrawFeePercent   *string     `json:"withdraw_fee_percent"`
	DepositOffStartedAt  *time.Time  `json:"deposit_off_started_at"`
	WithdrawOffStartedAt *time.Time  `json:"withdraw_off_started_at"`
	IsActive             bool        `json:"is_active"`
	MissingSince         *time.Time  `json:"missing_since"`
	MissingCount         int32       `json:"missing_count"`
	IsInitial            bool        `json:"is_initial"`
	LastSeenAt           time.Time   `json:"last_seen_at"`
}

type route struct {
	Exchange         exchangeRef `json:"exchange"`
	Coin             coinRef     `json:"coin"`
	FromCoin         coinRef     `json:"from_coin"`
	ToCoin           coinRef     `json:"to_coin"`
	FromChain        chainRef    `json:"from_chain"`
	ToChain          chainRef    `json:"to_chain"`
	DepositRail      rail        `json:"deposit_rail"`
	WithdrawRail     rail        `json:"withdraw_rail"`
	TotalFeeEstimate string      `json:"total_fee_estimate"`
	EquivalentAsset  bool        `json:"equivalent_asset"`
	RouteKind        string      `json:"route_kind"`

	totalFee decimal.Decimal
}

type routeOptions struct {
	Coin       coinRef           `json:"coin"`
	FromChains []chainRef        `json:"from_chains"`
	ToChains   []chainRef        `json:"to_chains"`
	Pairs      []routeOptionPair `json:"pairs"`
}

type routeOptionPair struct {
	FromChain chainRef `json:"from_chain"`
	ToChain   chainRef `json:"to_chain"`
}

type event struct {
	ID         int64              `json:"id"`
	RailID     int64              `json:"rail_id"`
	EventType  string             `json:"event_type"`
	Exchange   exchangeRef        `json:"exchange"`
	Coin       coinRef            `json:"coin"`
	Chain      chainRef           `json:"chain"`
	Before     json.RawMessage    `json:"before"`
	After      json.RawMessage    `json:"after"`
	Summary    string             `json:"summary"`
	Changes    []eventmeta.Change `json:"changes,omitempty"`
	OccurredAt time.Time          `json:"occurred_at"`
}

type listResponse[T any] struct {
	Data []T `json:"data"`
}

type paginatedResponse[T any] struct {
	Data       []T    `json:"data"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type errorResponse struct {
	Error responseError `json:"error"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
