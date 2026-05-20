package ws

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

type Cursor struct {
	OccurredAt time.Time `json:"occurred_at"`
	ID         int64     `json:"id"`
}

func EncodeCursor(cursor Cursor) (string, error) {
	if cursor.OccurredAt.IsZero() {
		return "", fmt.Errorf("cursor occurred_at is required")
	}
	if cursor.ID <= 0 {
		return "", fmt.Errorf("cursor id must be positive")
	}
	payload, err := json.Marshal(Cursor{
		OccurredAt: cursor.OccurredAt.UTC(),
		ID:         cursor.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecodeCursor(raw string) (Cursor, error) {
	if raw == "" {
		return Cursor{}, fmt.Errorf("cursor is empty")
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var cursor Cursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return Cursor{}, fmt.Errorf("parse cursor: %w", err)
	}
	if cursor.OccurredAt.IsZero() {
		return Cursor{}, fmt.Errorf("cursor occurred_at is required")
	}
	if cursor.ID <= 0 {
		return Cursor{}, fmt.Errorf("cursor id must be positive")
	}
	return Cursor{
		OccurredAt: cursor.OccurredAt.UTC(),
		ID:         cursor.ID,
	}, nil
}

func cursorFromEvent(event Event) Cursor {
	return Cursor{
		OccurredAt: event.OccurredAt,
		ID:         event.ID,
	}
}

func attachCursor(event Event) (Event, error) {
	cursor, err := EncodeCursor(cursorFromEvent(event))
	if err != nil {
		return Event{}, err
	}
	event.Cursor = cursor
	return event, nil
}
