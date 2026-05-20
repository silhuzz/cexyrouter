package rest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type railsCursor struct {
	CoinID     int32 `json:"coin_id"`
	ChainID    int32 `json:"chain_id"`
	ExchangeID int32 `json:"exchange_id"`
}

type eventsCursor struct {
	OccurredAt time.Time `json:"occurred_at"`
	ID         int64     `json:"id"`
}

func encodeRailsCursor(item rail) (string, error) {
	return encodeCursor(railsCursor{
		CoinID:     item.Coin.ID,
		ChainID:    item.Chain.ID,
		ExchangeID: item.Exchange.ID,
	})
}

func decodeRailsCursor(raw string) (railsCursor, error) {
	var cursor railsCursor
	if err := decodeCursor(raw, &cursor); err != nil {
		return railsCursor{}, err
	}
	if cursor.CoinID == 0 || cursor.ChainID == 0 || cursor.ExchangeID == 0 {
		return railsCursor{}, fmt.Errorf("cursor is missing rail key fields")
	}
	return cursor, nil
}

func encodeEventsCursor(item event) (string, error) {
	return encodeCursor(eventsCursor{
		OccurredAt: item.OccurredAt.UTC(),
		ID:         item.ID,
	})
}

func decodeEventsCursor(raw string) (eventsCursor, error) {
	var cursor eventsCursor
	if err := decodeCursor(raw, &cursor); err != nil {
		return eventsCursor{}, err
	}
	if cursor.OccurredAt.IsZero() || cursor.ID == 0 {
		return eventsCursor{}, fmt.Errorf("cursor is missing event key fields")
	}
	return cursor, nil
}

func encodeCursor(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(raw string, value any) error {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("decode cursor: %w", err)
	}
	if err := json.Unmarshal(payload, value); err != nil {
		return fmt.Errorf("parse cursor: %w", err)
	}
	return nil
}
