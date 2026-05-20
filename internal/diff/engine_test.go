package diff

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

var (
	testKey = Key{ExchangeID: 1, CoinID: 2, ChainID: 3}
	t0      = time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
)

func TestStateMachineRows(t *testing.T) {
	t.Run("no row plus present inserts initial rail without events", func(t *testing.T) {
		engine := NewEngine(Options{})

		result := mustDiff(t, engine, nil, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0, true, true)},
		})

		if len(result.Rails) != 1 {
			t.Fatalf("expected one rail mutation, got %d", len(result.Rails))
		}
		mutation := result.Rails[0]
		if mutation.Kind != MutationInsert {
			t.Fatalf("expected insert, got %s", mutation.Kind)
		}
		if !mutation.After.IsInitial {
			t.Fatalf("expected inserted rail to be initial")
		}
		if !mutation.After.IsActive {
			t.Fatalf("expected inserted rail to be active")
		}
		requireNoEvents(t, result)
	})

	t.Run("initial rail commits baseline on third consistent observation without events", func(t *testing.T) {
		engine := NewEngine(Options{})

		prev := latestRail(t, mustDiff(t, engine, nil, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0, true, true)},
		}))

		second := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(time.Minute), true, true)},
		})
		prev = latestRail(t, second)
		if !prev.IsInitial {
			t.Fatalf("expected second consistent observation to remain initial")
		}
		requireNoEvents(t, second)

		third := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(2*time.Minute), true, true)},
		})
		prev = latestRail(t, third)
		if prev.IsInitial {
			t.Fatalf("expected third consistent observation to commit baseline")
		}
		requireNoEvents(t, third)
	})

	t.Run("initial rail inconsistent observation resets baseline counter", func(t *testing.T) {
		engine := NewEngine(Options{})

		prev := latestRail(t, mustDiff(t, engine, nil, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0, true, true)},
		}))

		changed := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(time.Minute), false, true)},
		})
		prev = latestRail(t, changed)
		if !prev.IsInitial || prev.DepositEnabled {
			t.Fatalf("expected reset to current initial state, got is_initial=%v deposit=%v", prev.IsInitial, prev.DepositEnabled)
		}
		requireNoEvents(t, changed)

		secondConsistent := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(2*time.Minute), false, true)},
		})
		prev = latestRail(t, secondConsistent)
		if !prev.IsInitial {
			t.Fatalf("expected one post-reset confirmation to remain initial")
		}

		thirdConsistent := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(3*time.Minute), false, true)},
		})
		prev = latestRail(t, thirdConsistent)
		if prev.IsInitial {
			t.Fatalf("expected third matching observation after reset to commit baseline")
		}
		requireNoEvents(t, thirdConsistent)
	})

	t.Run("stable rail pending state change does not emit before three confirmations", func(t *testing.T) {
		engine := NewEngine(Options{})
		prev := stableRail(t0)

		first := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(time.Minute), false, true)},
		})
		prev = latestRail(t, first)
		if !prev.DepositEnabled {
			t.Fatalf("expected deposit status to stay stable before confirmation")
		}
		requireNoEvents(t, first)

		second := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(2*time.Minute), false, true)},
		})
		prev = latestRail(t, second)
		if !prev.DepositEnabled {
			t.Fatalf("expected deposit status to stay stable after two confirmations")
		}
		requireNoEvents(t, second)
	})

	t.Run("stable rail emits status event and updates state on third confirmation", func(t *testing.T) {
		engine := NewEngine(Options{})
		prev := stableRail(t0)
		firstObserved := t0.Add(time.Minute)

		for i := 0; i < 2; i++ {
			result := mustDiff(t, engine, []Rail{prev}, Poll{
				Complete:     true,
				Observations: []Observation{observationAt(firstObserved.Add(time.Duration(i)*time.Minute), false, true)},
			})
			prev = latestRail(t, result)
			requireNoEvents(t, result)
		}

		confirmed := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(firstObserved.Add(2*time.Minute), false, true)},
		})
		after := latestRail(t, confirmed)
		if after.DepositEnabled {
			t.Fatalf("expected confirmed deposit status to be off")
		}
		if after.DepositOffStartedAt == nil || !after.DepositOffStartedAt.Equal(firstObserved) {
			t.Fatalf("expected deposit off start at first pending observation, got %v", after.DepositOffStartedAt)
		}
		requireEventTypes(t, confirmed, EventDepositOff)
	})

	t.Run("fee and minimum changes update immediately with events", func(t *testing.T) {
		engine := NewEngine(Options{})
		prev := stableRail(t0)
		observation := observationAt(t0.Add(time.Minute), true, true)
		observation.WithdrawFee = decimalPtr("2.5")
		observation.WithdrawMin = decimalPtr("25")

		result := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observation},
		})

		after := latestRail(t, result)
		if !after.WithdrawFee.Equal(decimal.RequireFromString("2.5")) {
			t.Fatalf("expected fee update, got %v", after.WithdrawFee)
		}
		if !after.WithdrawMin.Equal(decimal.RequireFromString("25")) {
			t.Fatalf("expected min update, got %v", after.WithdrawMin)
		}
		requireEventTypes(t, result, EventFeeChanged, EventMinChanged)
	})

	t.Run("partial poll with absent rail does not increment missing count or delist", func(t *testing.T) {
		engine := NewEngine(Options{DelistThreshold: 1})
		prev := stableRail(t0)

		result := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:   false,
			ObservedAt: t0.Add(time.Minute),
		})

		if !result.Empty() {
			t.Fatalf("expected no mutations or events for absent rail in partial poll: %+v", result)
		}
	})

	t.Run("complete poll absent rail delists at threshold", func(t *testing.T) {
		engine := NewEngine(Options{DelistThreshold: 2})
		prev := stableRail(t0)

		first := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:   true,
			ObservedAt: t0.Add(time.Minute),
		})
		prev = latestRail(t, first)
		if prev.MissingCount != 1 || !prev.IsActive {
			t.Fatalf("expected first absence to increment missing only, got count=%d active=%v", prev.MissingCount, prev.IsActive)
		}
		requireNoEvents(t, first)

		secondAt := t0.Add(2 * time.Minute)
		second := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:   true,
			ObservedAt: secondAt,
		})
		prev = latestRail(t, second)
		if prev.IsActive || prev.MissingCount != 2 {
			t.Fatalf("expected threshold absence to delist, got count=%d active=%v", prev.MissingCount, prev.IsActive)
		}
		if prev.MissingSince == nil || !prev.MissingSince.Equal(secondAt) {
			t.Fatalf("expected missing_since at threshold time, got %v", prev.MissingSince)
		}
		requireEventTypes(t, second, EventRailDelisted)
	})

	t.Run("inactive rail present in complete poll relists immediately", func(t *testing.T) {
		engine := NewEngine(Options{})
		missingSince := t0.Add(-time.Minute)
		prev := stableRail(t0)
		prev.IsActive = false
		prev.MissingCount = 6
		prev.MissingSince = &missingSince

		result := mustDiff(t, engine, []Rail{prev}, Poll{
			Complete:     true,
			Observations: []Observation{observationAt(t0.Add(time.Minute), true, true)},
		})

		after := latestRail(t, result)
		if !after.IsActive || after.MissingCount != 0 || after.MissingSince != nil {
			t.Fatalf("expected relisted rail to be active with missing cleared: %+v", after)
		}
		requireEventTypes(t, result, EventRailRelisted)
	})
}

func TestApplyUsesAbstractWriter(t *testing.T) {
	expectedErr := errors.New("boom")
	if err := Apply(context.Background(), nil, Result{}); !errors.Is(err, ErrNilWriter) {
		t.Fatalf("expected nil writer error, got %v", err)
	}

	writer := &recordingWriter{}
	result := Result{
		Rails: []RailMutation{{
			Kind:  MutationInsert,
			After: stableRail(t0),
		}},
		Events: []Event{{
			Type:       EventRailRelisted,
			Key:        testKey,
			OccurredAt: t0,
		}},
	}
	if err := Apply(context.Background(), writer, result); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !reflect.DeepEqual(writer.calls, []string{"upsert", "events"}) {
		t.Fatalf("unexpected call order: %v", writer.calls)
	}

	writer = &recordingWriter{upsertErr: expectedErr}
	if err := Apply(context.Background(), writer, result); !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped upsert error, got %v", err)
	}
}

type recordingWriter struct {
	calls     []string
	upsertErr error
	eventsErr error
}

func (w *recordingWriter) UpsertRails(_ context.Context, _ []RailMutation) error {
	w.calls = append(w.calls, "upsert")
	return w.upsertErr
}

func (w *recordingWriter) InsertEvents(_ context.Context, _ []Event) error {
	w.calls = append(w.calls, "events")
	return w.eventsErr
}

func mustDiff(t *testing.T, engine *Engine, previous []Rail, current Poll) Result {
	t.Helper()
	result, err := engine.Diff(previous, current)
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}
	return result
}

func latestRail(t *testing.T, result Result) Rail {
	t.Helper()
	if len(result.Rails) == 0 {
		t.Fatalf("expected at least one rail mutation")
	}
	return result.Rails[len(result.Rails)-1].After
}

func requireNoEvents(t *testing.T, result Result) {
	t.Helper()
	if len(result.Events) != 0 {
		t.Fatalf("expected no events, got %+v", result.Events)
	}
}

func requireEventTypes(t *testing.T, result Result, expected ...EventType) {
	t.Helper()
	if len(result.Events) != len(expected) {
		t.Fatalf("expected events %v, got %+v", expected, result.Events)
	}
	for i, eventType := range expected {
		if result.Events[i].Type != eventType {
			t.Fatalf("event %d: expected %s, got %s", i, eventType, result.Events[i].Type)
		}
	}
}

func stableRail(lastSeen time.Time) Rail {
	return Rail{
		ID:                   42,
		Key:                  testKey,
		DepositEnabled:       true,
		WithdrawEnabled:      true,
		DepositConfirmations: intPtr(12),
		WithdrawMin:          decimalPtr("10"),
		WithdrawFee:          decimalPtr("1"),
		WithdrawFeeType:      "fixed",
		IsActive:             true,
		IsInitial:            false,
		LastSeenAt:           lastSeen,
	}
}

func observationAt(observedAt time.Time, depositEnabled bool, withdrawEnabled bool) Observation {
	return Observation{
		Key:                  testKey,
		DepositEnabled:       depositEnabled,
		WithdrawEnabled:      withdrawEnabled,
		DepositConfirmations: intPtr(12),
		WithdrawMin:          decimalPtr("10"),
		WithdrawFee:          decimalPtr("1"),
		WithdrawFeeType:      "fixed",
		ObservedAt:           observedAt,
	}
}

func intPtr(value int) *int {
	return &value
}

func decimalPtr(value string) *decimal.Decimal {
	parsed := decimal.RequireFromString(value)
	return &parsed
}
