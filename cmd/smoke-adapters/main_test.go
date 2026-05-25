package main

import "testing"

func TestParseWantedNormalizesGateIOAlias(t *testing.T) {
	wanted := parseWanted("gate.io,bitget")
	if !wanted["gate"] {
		t.Fatalf("wanted[gate] = false, want true")
	}
	if wanted["gate.io"] {
		t.Fatalf("wanted[gate.io] = true, want normalized slug only")
	}
	if !wanted["bitget"] {
		t.Fatalf("wanted[bitget] = false, want true")
	}
}
