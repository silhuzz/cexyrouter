package types

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

const (
	FeeTypeFixed   = "fixed"
	FeeTypePercent = "percent"
	FeeTypeHybrid  = "hybrid"
)

const (
	EventDepositOff   = "deposit_off"
	EventDepositOn    = "deposit_on"
	EventWithdrawOff  = "withdraw_off"
	EventWithdrawOn   = "withdraw_on"
	EventFeeChanged   = "fee_changed"
	EventMinChanged   = "min_changed"
	EventRailDelisted = "rail_delisted"
	EventRailRelisted = "rail_relisted"
)

type RailSnapshot struct {
	ExchangeSlug         string
	CoinSymbol           string
	CoinName             string
	RawChainSymbol       string
	RawChainName         string
	RawNetworkID         string
	ContractAddress      string
	DepositEnabled       bool
	WithdrawEnabled      bool
	DepositConfirmations *int
	WithdrawMin          *decimal.Decimal
	WithdrawFee          *decimal.Decimal
	WithdrawFeeType      string
	WithdrawFeePercent   *decimal.Decimal
	ObservedAt           time.Time
}

type FetchResult struct {
	Snapshots []RailSnapshot
	Complete  bool
	Notes     []string
}

type Adapter interface {
	Slug() string
	FetchRails(ctx context.Context) (FetchResult, error)
}
