package eventmeta

import (
	"encoding/json"
	"testing"
)

func TestBuildFeeChangedShowsDelta(t *testing.T) {
	details := Build("fee_changed",
		json.RawMessage(`{"WithdrawFee":"1.25","WithdrawFeeType":"fixed"}`),
		json.RawMessage(`{"WithdrawFee":"2.50","WithdrawFeeType":"fixed"}`),
	)

	if details.Summary != "Withdraw fee 1.25 -> 2.5 (+1.25, +100%)" {
		t.Fatalf("summary = %q", details.Summary)
	}
	if len(details.Changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(details.Changes))
	}
	if details.Changes[0].Field != "withdraw_fee" || details.Changes[0].Delta != "+1.25" {
		t.Fatalf("unexpected change: %+v", details.Changes[0])
	}
}

func TestBuildMinChangedAcceptsSnakeCase(t *testing.T) {
	details := Build("min_changed",
		json.RawMessage(`{"withdraw_min":"10"}`),
		json.RawMessage(`{"withdraw_min":"8"}`),
	)

	if details.Summary != "Withdraw min 10 -> 8 (-2, -20%)" {
		t.Fatalf("summary = %q", details.Summary)
	}
	if details.Changes[0].Direction != "down" {
		t.Fatalf("direction = %q, want down", details.Changes[0].Direction)
	}
}

func TestBuildWithdrawOffShowsStatus(t *testing.T) {
	details := Build("withdraw_off",
		json.RawMessage(`{"WithdrawEnabled":true}`),
		json.RawMessage(`{"WithdrawEnabled":false}`),
	)

	if details.Summary != "Withdraw on -> off" {
		t.Fatalf("summary = %q", details.Summary)
	}
}
