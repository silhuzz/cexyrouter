package diff

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/silhuzz/cexyrouter/pkg/types"
)

type EventType string

const (
	EventDepositOff   EventType = types.EventDepositOff
	EventDepositOn    EventType = types.EventDepositOn
	EventWithdrawOff  EventType = types.EventWithdrawOff
	EventWithdrawOn   EventType = types.EventWithdrawOn
	EventFeeChanged   EventType = types.EventFeeChanged
	EventMinChanged   EventType = types.EventMinChanged
	EventRailDelisted EventType = types.EventRailDelisted
	EventRailRelisted EventType = types.EventRailRelisted
)

type Key struct {
	ExchangeID int64
	CoinID     int64
	ChainID    int64
}

type Observation struct {
	Key
	DepositEnabled       bool
	WithdrawEnabled      bool
	DepositConfirmations *int
	WithdrawMin          *decimal.Decimal
	WithdrawFee          *decimal.Decimal
	WithdrawFeeType      string
	WithdrawFeePercent   *decimal.Decimal
	ContractAddress      string
	ObservedAt           time.Time
}

type Poll struct {
	Observations []Observation
	Complete     bool
	ObservedAt   time.Time
}

type Rail struct {
	ID                   int64
	Key                  Key
	DepositEnabled       bool
	WithdrawEnabled      bool
	DepositConfirmations *int
	WithdrawMin          *decimal.Decimal
	WithdrawFee          *decimal.Decimal
	WithdrawFeeType      string
	WithdrawFeePercent   *decimal.Decimal
	ContractAddress      string
	DepositOffStartedAt  *time.Time
	WithdrawOffStartedAt *time.Time
	IsActive             bool
	MissingSince         *time.Time
	MissingCount         int
	IsInitial            bool
	LastSeenAt           time.Time
}

type RailState struct {
	DepositEnabled       bool
	WithdrawEnabled      bool
	DepositConfirmations *int
	WithdrawMin          *decimal.Decimal
	WithdrawFee          *decimal.Decimal
	WithdrawFeeType      string
	WithdrawFeePercent   *decimal.Decimal
	DepositOffStartedAt  *time.Time
	WithdrawOffStartedAt *time.Time
	IsActive             bool
	MissingSince         *time.Time
	MissingCount         int
	IsInitial            bool
	LastSeenAt           time.Time
}

type Event struct {
	Type       EventType
	RailID     int64
	Key        Key
	Before     RailState
	After      RailState
	OccurredAt time.Time
}

type MutationKind string

const (
	MutationInsert MutationKind = "insert"
	MutationUpdate MutationKind = "update"
)

type RailMutation struct {
	Kind   MutationKind
	Before *Rail
	After  Rail
}

type Result struct {
	Rails  []RailMutation
	Events []Event
}

func (r Result) Empty() bool {
	return len(r.Rails) == 0 && len(r.Events) == 0
}

func StateFromRail(rail Rail) RailState {
	return RailState{
		DepositEnabled:       rail.DepositEnabled,
		WithdrawEnabled:      rail.WithdrawEnabled,
		DepositConfirmations: cloneIntPtr(rail.DepositConfirmations),
		WithdrawMin:          cloneDecimalPtr(rail.WithdrawMin),
		WithdrawFee:          cloneDecimalPtr(rail.WithdrawFee),
		WithdrawFeeType:      rail.WithdrawFeeType,
		WithdrawFeePercent:   cloneDecimalPtr(rail.WithdrawFeePercent),
		DepositOffStartedAt:  cloneTimePtr(rail.DepositOffStartedAt),
		WithdrawOffStartedAt: cloneTimePtr(rail.WithdrawOffStartedAt),
		IsActive:             rail.IsActive,
		MissingSince:         cloneTimePtr(rail.MissingSince),
		MissingCount:         rail.MissingCount,
		IsInitial:            rail.IsInitial,
		LastSeenAt:           rail.LastSeenAt,
	}
}
