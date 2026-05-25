package ws

import (
	"encoding/json"
	"time"

	"github.com/silhuzz/cexyrouter/internal/api/eventmeta"
)

const (
	defaultOutboundBuffer = 64
	slowClientCloseCode   = 1013
	backfillBatchLimit    = 1000
	maxBackfillWindow     = 24 * time.Hour
)

type Ref struct {
	ID   int32  `json:"id"`
	Slug string `json:"slug"`
}

type ExchangeRef struct {
	Ref
	Name   string `json:"name"`
	Region string `json:"region"`
}

type CoinRef struct {
	Ref
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

type ChainRef struct {
	Ref
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	EVMChainID    *int32 `json:"evm_chain_id"`
	ParentChainID *int32 `json:"parent_chain_id"`
}

type Event struct {
	ID         int64              `json:"id"`
	RailID     int64              `json:"rail_id"`
	EventType  string             `json:"event_type"`
	Exchange   ExchangeRef        `json:"exchange"`
	Coin       CoinRef            `json:"coin"`
	Chain      ChainRef           `json:"chain"`
	Before     json.RawMessage    `json:"before"`
	After      json.RawMessage    `json:"after"`
	Summary    string             `json:"summary"`
	Changes    []eventmeta.Change `json:"changes,omitempty"`
	OccurredAt time.Time          `json:"occurred_at"`
	Cursor     string             `json:"cursor,omitempty"`
}

type envelope struct {
	Type  string `json:"type"`
	Event Event  `json:"event"`
}

type errorResponse struct {
	Error responseError `json:"error"`
}

type responseError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}
