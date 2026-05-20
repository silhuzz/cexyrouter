package commands

import (
	"reflect"
	"testing"
)

func TestParseSubscribeNormalizesFiltersAndEvents(t *testing.T) {
	cmd, err := Parse("/subscribe OKX USDT TRON withdraw_off,withdraw_on")
	if err != nil {
		t.Fatalf("parse subscribe: %v", err)
	}
	if cmd.Kind != KindSubscribe {
		t.Fatalf("kind = %s, want %s", cmd.Kind, KindSubscribe)
	}
	if cmd.Subscribe.Exchange != "okx" || cmd.Subscribe.Coin != "usdt" || cmd.Subscribe.Chain != "tron" {
		t.Fatalf("unexpected filters: %+v", cmd.Subscribe)
	}
	wantEvents := []string{"withdraw_off", "withdraw_on"}
	if !reflect.DeepEqual(cmd.Subscribe.EventTypes, wantEvents) {
		t.Fatalf("event types = %#v, want %#v", cmd.Subscribe.EventTypes, wantEvents)
	}
}

func TestParseSubscribeWildcardHandling(t *testing.T) {
	cmd, err := Parse("/subscribe * USDT * *")
	if err != nil {
		t.Fatalf("parse subscribe: %v", err)
	}
	if !IsWildcard(cmd.Subscribe.Exchange) {
		t.Fatalf("exchange wildcard normalized to %q", cmd.Subscribe.Exchange)
	}
	if cmd.Subscribe.Coin != "usdt" {
		t.Fatalf("coin = %q, want usdt", cmd.Subscribe.Coin)
	}
	if !IsWildcard(cmd.Subscribe.Chain) {
		t.Fatalf("chain wildcard normalized to %q", cmd.Subscribe.Chain)
	}
	if !reflect.DeepEqual(cmd.Subscribe.EventTypes, DefaultEventTypes()) {
		t.Fatalf("event wildcard = %#v, want all events", cmd.Subscribe.EventTypes)
	}
}

func TestParseStatusWildcardHandling(t *testing.T) {
	cmd, err := Parse("/status * USDC polygon")
	if err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if !IsWildcard(cmd.Status.Exchange) {
		t.Fatalf("exchange wildcard normalized to %q", cmd.Status.Exchange)
	}
	if cmd.Status.Coin != "usdc" || cmd.Status.Chain != "polygon" {
		t.Fatalf("unexpected status filter: %+v", cmd.Status)
	}
}

func TestParseUnsubscribe(t *testing.T) {
	cmd, err := Parse("/unsubscribe 42")
	if err != nil {
		t.Fatalf("parse unsubscribe: %v", err)
	}
	if cmd.Kind != KindUnsubscribe || cmd.RuleID != 42 {
		t.Fatalf("unexpected command: %+v", cmd)
	}
}

func TestParseRejectsUnknownEventType(t *testing.T) {
	if _, err := Parse("/subscribe okx usdt tron withdraw_broken"); err == nil {
		t.Fatal("expected unknown event type error")
	}
}
