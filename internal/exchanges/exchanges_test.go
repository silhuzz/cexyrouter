package exchanges

import (
	"reflect"
	"testing"
)

func TestNormalizeSlug(t *testing.T) {
	tests := map[string]string{
		"Gate.io": "gate",
		"gateio":  "gate",
		"gate io": "gate",
		"Bybit":   "bybit",
	}

	for raw, want := range tests {
		if got := NormalizeSlug(raw); got != want {
			t.Fatalf("NormalizeSlug(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeSlugsDeduplicates(t *testing.T) {
	got := NormalizeSlugs([]string{"gate.io", "gateio", "bitget", "bitget"})
	want := []string{"gate", "bitget"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizeSlugs() = %#v, want %#v", got, want)
	}
}
