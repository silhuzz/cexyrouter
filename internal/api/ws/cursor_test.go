package ws

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	occurredAt := testTime()
	raw, err := EncodeCursor(Cursor{
		OccurredAt: occurredAt,
		ID:         42,
	})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}

	cursor, err := DecodeCursor(raw)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if cursor.ID != 42 || !cursor.OccurredAt.Equal(occurredAt) {
		t.Fatalf("cursor = %+v, want id=42 occurred_at=%s", cursor, occurredAt)
	}
}

func TestDecodeCursorRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "empty", raw: ""},
		{name: "bad base64", raw: "not cursor"},
		{name: "bad json", raw: base64.RawURLEncoding.EncodeToString([]byte(`{`))},
		{name: "missing occurred_at", raw: mustCursorPayload(t, map[string]any{"id": 1})},
		{name: "missing id", raw: mustCursorPayload(t, map[string]any{"occurred_at": testTime()})},
		{name: "zero id", raw: mustCursorPayload(t, map[string]any{"occurred_at": testTime(), "id": 0})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodeCursor(tt.raw); err == nil {
				t.Fatalf("expected DecodeCursor to reject %q", tt.raw)
			}
		})
	}
}

func mustCursorPayload(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func testTime() time.Time {
	return time.Date(2026, 5, 20, 12, 34, 56, 789, time.UTC)
}
