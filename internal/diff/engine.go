package diff

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
)

const (
	DefaultConfirmationsRequired = 3
	DefaultDelistThreshold       = 6
)

var ErrNilEngine = errors.New("diff engine is nil")

type Options struct {
	ConfirmationsRequired int
	DelistThreshold       int
}

type Engine struct {
	opts     Options
	baseline map[Key]pendingBaseline
	status   map[Key]pendingStatus
}

type pendingBaseline struct {
	target statusState
	count  int
}

type pendingStatus struct {
	target          statusState
	count           int
	firstObservedAt time.Time
}

type statusState struct {
	depositEnabled  bool
	withdrawEnabled bool
}

func NewEngine(opts Options) *Engine {
	return &Engine{
		opts:     opts.withDefaults(),
		baseline: make(map[Key]pendingBaseline),
		status:   make(map[Key]pendingStatus),
	}
}

func Diff(previous []Rail, current Poll) (Result, error) {
	return NewEngine(Options{}).Diff(previous, current)
}

func (e *Engine) Diff(previous []Rail, current Poll) (Result, error) {
	if e == nil {
		return Result{}, ErrNilEngine
	}

	opts := e.opts.withDefaults()
	prevByKey, err := railsByKey(previous)
	if err != nil {
		return Result{}, err
	}

	seen := make(map[Key]struct{}, len(current.Observations))
	result := Result{}
	for _, observation := range current.Observations {
		if _, ok := seen[observation.Key]; ok {
			return Result{}, fmt.Errorf("duplicate observation for key %+v", observation.Key)
		}
		seen[observation.Key] = struct{}{}
		observation.ObservedAt = observationTime(observation, current)

		prev, exists := prevByKey[observation.Key]
		if !exists {
			e.insertInitial(&result, observation)
			continue
		}

		e.handlePresent(&result, prev, observation, current.Complete, opts)
	}

	if current.Complete {
		for _, key := range sortedAbsentKeys(prevByKey, seen) {
			e.handleAbsent(&result, prevByKey[key], current.ObservedAt, opts)
		}
	}

	return result, nil
}

func (e *Engine) insertInitial(result *Result, observation Observation) {
	after := railFromObservation(observation)
	after.IsActive = true
	after.IsInitial = true
	after.MissingCount = 0
	after.MissingSince = nil
	result.Rails = append(result.Rails, RailMutation{
		Kind:  MutationInsert,
		After: after,
	})

	e.baseline[observation.Key] = pendingBaseline{
		target: statusFromObservation(observation),
		count:  1,
	}
	delete(e.status, observation.Key)
}

func (e *Engine) handlePresent(result *Result, prev Rail, observation Observation, complete bool, opts Options) {
	if !prev.IsInitial && !prev.IsActive && complete {
		e.handleRelisted(result, prev, observation)
		return
	}

	if prev.IsInitial {
		e.handleInitial(result, prev, observation, opts)
		return
	}

	if !prev.IsActive {
		after := prev
		applyObservation(&after, observation, true)
		if !railsEqual(prev, after) {
			result.Rails = append(result.Rails, updateMutation(prev, after))
		}
		return
	}

	e.handleActive(result, prev, observation, opts)
}

func (e *Engine) handleInitial(result *Result, prev Rail, observation Observation, opts Options) {
	target := statusFromObservation(observation)
	pending := e.baseline[prev.Key]
	if pending.target != target {
		pending = pendingBaseline{target: target}
	}
	pending.count++

	after := prev
	applyObservation(&after, observation, true)
	after.IsActive = prev.IsActive
	after.IsInitial = true
	after.MissingCount = 0
	after.MissingSince = nil

	if pending.count >= opts.ConfirmationsRequired {
		after.IsInitial = false
		delete(e.baseline, prev.Key)
	} else {
		e.baseline[prev.Key] = pending
	}
	delete(e.status, prev.Key)

	if !railsEqual(prev, after) {
		result.Rails = append(result.Rails, updateMutation(prev, after))
	}
}

func (e *Engine) handleActive(result *Result, prev Rail, observation Observation, opts Options) {
	after := prev
	applyObservation(&after, observation, false)
	after.MissingCount = 0
	after.MissingSince = nil
	after.IsActive = true
	after.IsInitial = false

	feeChanged := feeChanged(prev, observation)
	minChanged := minChanged(prev, observation)

	currentStatus := statusFromRail(prev)
	target := statusFromObservation(observation)
	var statusEvents []EventType
	if target == currentStatus {
		delete(e.status, prev.Key)
	} else {
		pending, ok := e.status[prev.Key]
		if !ok || pending.target != target {
			pending = pendingStatus{
				target:          target,
				firstObservedAt: observation.ObservedAt,
			}
		}
		pending.count++

		if pending.count >= opts.ConfirmationsRequired {
			applyStatus(&after, target, pending.firstObservedAt)
			statusEvents = statusChangeEvents(prev, after)
			delete(e.status, prev.Key)
		} else {
			e.status[prev.Key] = pending
		}
	}

	if !railsEqual(prev, after) {
		result.Rails = append(result.Rails, updateMutation(prev, after))
	}

	for _, eventType := range statusEvents {
		result.Events = append(result.Events, newEvent(eventType, prev, after, observation.ObservedAt))
	}
	if feeChanged {
		result.Events = append(result.Events, newEvent(EventFeeChanged, prev, after, observation.ObservedAt))
	}
	if minChanged {
		result.Events = append(result.Events, newEvent(EventMinChanged, prev, after, observation.ObservedAt))
	}
}

func (e *Engine) handleAbsent(result *Result, prev Rail, observedAt time.Time, opts Options) {
	delete(e.baseline, prev.Key)
	delete(e.status, prev.Key)

	after := prev
	after.MissingCount++
	if prev.IsActive && after.MissingCount >= opts.DelistThreshold {
		after.IsActive = false
		after.MissingSince = cloneTime(observedAt)
	}

	if railsEqual(prev, after) {
		return
	}

	result.Rails = append(result.Rails, updateMutation(prev, after))
	if prev.IsActive && !after.IsActive && !prev.IsInitial {
		result.Events = append(result.Events, newEvent(EventRailDelisted, prev, after, observedAt))
	}
}

func (e *Engine) handleRelisted(result *Result, prev Rail, observation Observation) {
	delete(e.baseline, prev.Key)
	delete(e.status, prev.Key)

	after := prev
	applyObservation(&after, observation, true)
	after.IsActive = true
	after.IsInitial = false
	after.MissingCount = 0
	after.MissingSince = nil

	if !railsEqual(prev, after) {
		result.Rails = append(result.Rails, updateMutation(prev, after))
	}
	result.Events = append(result.Events, newEvent(EventRailRelisted, prev, after, observation.ObservedAt))
}

func railFromObservation(observation Observation) Rail {
	rail := Rail{
		Key:                  observation.Key,
		DepositEnabled:       observation.DepositEnabled,
		WithdrawEnabled:      observation.WithdrawEnabled,
		DepositConfirmations: cloneIntPtr(observation.DepositConfirmations),
		WithdrawMin:          cloneDecimalPtr(observation.WithdrawMin),
		WithdrawFee:          cloneDecimalPtr(observation.WithdrawFee),
		WithdrawFeeType:      observation.WithdrawFeeType,
		WithdrawFeePercent:   cloneDecimalPtr(observation.WithdrawFeePercent),
		ContractAddress:      observation.ContractAddress,
		LastSeenAt:           observation.ObservedAt,
	}
	if !observation.DepositEnabled {
		rail.DepositOffStartedAt = cloneTime(observation.ObservedAt)
	}
	if !observation.WithdrawEnabled {
		rail.WithdrawOffStartedAt = cloneTime(observation.ObservedAt)
	}
	return rail
}

func applyObservation(rail *Rail, observation Observation, includeStatus bool) {
	if includeStatus {
		applyStatus(rail, statusFromObservation(observation), observation.ObservedAt)
	}
	rail.DepositConfirmations = cloneIntPtr(observation.DepositConfirmations)
	rail.WithdrawMin = cloneDecimalPtr(observation.WithdrawMin)
	rail.WithdrawFee = cloneDecimalPtr(observation.WithdrawFee)
	rail.WithdrawFeeType = observation.WithdrawFeeType
	rail.WithdrawFeePercent = cloneDecimalPtr(observation.WithdrawFeePercent)
	rail.ContractAddress = observation.ContractAddress
	rail.LastSeenAt = observation.ObservedAt
}

func applyStatus(rail *Rail, target statusState, changedAt time.Time) {
	if rail.DepositEnabled != target.depositEnabled {
		if target.depositEnabled {
			rail.DepositOffStartedAt = nil
		} else {
			rail.DepositOffStartedAt = cloneTime(changedAt)
		}
		rail.DepositEnabled = target.depositEnabled
	} else if !target.depositEnabled && rail.DepositOffStartedAt == nil {
		rail.DepositOffStartedAt = cloneTime(changedAt)
	}

	if rail.WithdrawEnabled != target.withdrawEnabled {
		if target.withdrawEnabled {
			rail.WithdrawOffStartedAt = nil
		} else {
			rail.WithdrawOffStartedAt = cloneTime(changedAt)
		}
		rail.WithdrawEnabled = target.withdrawEnabled
	} else if !target.withdrawEnabled && rail.WithdrawOffStartedAt == nil {
		rail.WithdrawOffStartedAt = cloneTime(changedAt)
	}
}

func statusChangeEvents(before, after Rail) []EventType {
	events := make([]EventType, 0, 2)
	if before.DepositEnabled != after.DepositEnabled {
		if after.DepositEnabled {
			events = append(events, EventDepositOn)
		} else {
			events = append(events, EventDepositOff)
		}
	}
	if before.WithdrawEnabled != after.WithdrawEnabled {
		if after.WithdrawEnabled {
			events = append(events, EventWithdrawOn)
		} else {
			events = append(events, EventWithdrawOff)
		}
	}
	return events
}

func newEvent(eventType EventType, before, after Rail, occurredAt time.Time) Event {
	return Event{
		Type:       eventType,
		RailID:     before.ID,
		Key:        before.Key,
		Before:     StateFromRail(before),
		After:      StateFromRail(after),
		OccurredAt: occurredAt,
	}
}

func updateMutation(before, after Rail) RailMutation {
	beforeCopy := before
	return RailMutation{
		Kind:   MutationUpdate,
		Before: &beforeCopy,
		After:  after,
	}
}

func railsByKey(rails []Rail) (map[Key]Rail, error) {
	byKey := make(map[Key]Rail, len(rails))
	for _, rail := range rails {
		if _, ok := byKey[rail.Key]; ok {
			return nil, fmt.Errorf("duplicate previous rail for key %+v", rail.Key)
		}
		byKey[rail.Key] = rail
	}
	return byKey, nil
}

func sortedAbsentKeys(previous map[Key]Rail, seen map[Key]struct{}) []Key {
	keys := make([]Key, 0)
	for key := range previous {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ExchangeID != keys[j].ExchangeID {
			return keys[i].ExchangeID < keys[j].ExchangeID
		}
		if keys[i].CoinID != keys[j].CoinID {
			return keys[i].CoinID < keys[j].CoinID
		}
		return keys[i].ChainID < keys[j].ChainID
	})
	return keys
}

func observationTime(observation Observation, poll Poll) time.Time {
	if !observation.ObservedAt.IsZero() {
		return observation.ObservedAt
	}
	return poll.ObservedAt
}

func statusFromRail(rail Rail) statusState {
	return statusState{
		depositEnabled:  rail.DepositEnabled,
		withdrawEnabled: rail.WithdrawEnabled,
	}
}

func statusFromObservation(observation Observation) statusState {
	return statusState{
		depositEnabled:  observation.DepositEnabled,
		withdrawEnabled: observation.WithdrawEnabled,
	}
}

func feeChanged(prev Rail, observation Observation) bool {
	return !decimalPtrEqual(prev.WithdrawFee, observation.WithdrawFee) ||
		prev.WithdrawFeeType != observation.WithdrawFeeType ||
		!decimalPtrEqual(prev.WithdrawFeePercent, observation.WithdrawFeePercent)
}

func minChanged(prev Rail, observation Observation) bool {
	return !decimalPtrEqual(prev.WithdrawMin, observation.WithdrawMin)
}

func (opts Options) withDefaults() Options {
	if opts.ConfirmationsRequired <= 0 {
		opts.ConfirmationsRequired = DefaultConfirmationsRequired
	}
	if opts.DelistThreshold <= 0 {
		opts.DelistThreshold = DefaultDelistThreshold
	}
	return opts
}

func railsEqual(a, b Rail) bool {
	return a.ID == b.ID &&
		a.Key == b.Key &&
		a.DepositEnabled == b.DepositEnabled &&
		a.WithdrawEnabled == b.WithdrawEnabled &&
		intPtrEqual(a.DepositConfirmations, b.DepositConfirmations) &&
		decimalPtrEqual(a.WithdrawMin, b.WithdrawMin) &&
		decimalPtrEqual(a.WithdrawFee, b.WithdrawFee) &&
		a.WithdrawFeeType == b.WithdrawFeeType &&
		decimalPtrEqual(a.WithdrawFeePercent, b.WithdrawFeePercent) &&
		a.ContractAddress == b.ContractAddress &&
		timePtrEqual(a.DepositOffStartedAt, b.DepositOffStartedAt) &&
		timePtrEqual(a.WithdrawOffStartedAt, b.WithdrawOffStartedAt) &&
		a.IsActive == b.IsActive &&
		timePtrEqual(a.MissingSince, b.MissingSince) &&
		a.MissingCount == b.MissingCount &&
		a.IsInitial == b.IsInitial &&
		a.LastSeenAt.Equal(b.LastSeenAt)
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneDecimalPtr(value *decimal.Decimal) *decimal.Decimal {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func decimalPtrEqual(a, b *decimal.Decimal) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}
