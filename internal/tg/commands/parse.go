package commands

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	exchangeinfo "github.com/silhuzz/cexyrouter/internal/exchanges"
)

type Kind string

const (
	KindStart       Kind = "start"
	KindHelp        Kind = "help"
	KindSubscribe   Kind = "subscribe"
	KindList        Kind = "list"
	KindUnsubscribe Kind = "unsubscribe"
	KindStatus      Kind = "status"
)

var (
	ErrNotCommand = errors.New("message is not a command")

	allEventTypes = []string{
		"deposit_off",
		"deposit_on",
		"withdraw_off",
		"withdraw_on",
		"fee_changed",
		"min_changed",
		"rail_delisted",
		"rail_relisted",
	}

	availabilityEventTypes = []string{
		"deposit_off",
		"deposit_on",
		"withdraw_off",
		"withdraw_on",
		"rail_delisted",
		"rail_relisted",
	}
)

type Command struct {
	Kind         Kind
	Subscribe    SubscribeCommand
	Status       StatusCommand
	RuleID       int64
	OriginalText string
}

type SubscribeCommand struct {
	Exchange   string
	Coin       string
	Chain      string
	EventTypes []string
}

type StatusCommand struct {
	Exchange string
	Coin     string
	Chain    string
}

func Parse(text string) (Command, error) {
	original := strings.TrimSpace(text)
	if original == "" {
		return Command{}, ErrNotCommand
	}

	fields := strings.Fields(original)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return Command{}, ErrNotCommand
	}

	name := strings.TrimPrefix(fields[0], "/")
	if at := strings.IndexByte(name, '@'); at >= 0 {
		name = name[:at]
	}
	name = strings.ToLower(strings.TrimSpace(name))

	cmd := Command{Kind: Kind(name), OriginalText: original}
	args := fields[1:]
	switch cmd.Kind {
	case KindStart, KindHelp, KindList:
		return cmd, nil
	case KindSubscribe:
		sub, err := parseSubscribe(args)
		if err != nil {
			return Command{}, err
		}
		cmd.Subscribe = sub
		return cmd, nil
	case KindUnsubscribe:
		ruleID, err := parseRuleID(args)
		if err != nil {
			return Command{}, err
		}
		cmd.RuleID = ruleID
		return cmd, nil
	case KindStatus:
		status, err := parseStatus(args)
		if err != nil {
			return Command{}, err
		}
		cmd.Status = status
		return cmd, nil
	default:
		return Command{}, fmt.Errorf("unknown command %q", fields[0])
	}
}

func parseSubscribe(args []string) (SubscribeCommand, error) {
	if len(args) > 4 {
		args = append(args[:3], strings.Join(args[3:], ""))
	}

	sub := SubscribeCommand{
		EventTypes: DefaultEventTypes(),
	}
	if len(args) > 0 {
		sub.Exchange = normalizeExchangeFilter(args[0])
	}
	if len(args) > 1 {
		sub.Coin = normalizeFilter(args[1])
	}
	if len(args) > 2 {
		sub.Chain = normalizeFilter(args[2])
	}
	if len(args) > 3 {
		eventTypes, err := parseEventTypes(args[3])
		if err != nil {
			return SubscribeCommand{}, err
		}
		sub.EventTypes = eventTypes
	}
	return sub, nil
}

func parseStatus(args []string) (StatusCommand, error) {
	if len(args) > 3 {
		return StatusCommand{}, errors.New("usage: /status [exchange] [coin] [chain]")
	}

	var status StatusCommand
	if len(args) > 0 {
		status.Exchange = normalizeExchangeFilter(args[0])
	}
	if len(args) > 1 {
		status.Coin = normalizeFilter(args[1])
	}
	if len(args) > 2 {
		status.Chain = normalizeFilter(args[2])
	}
	return status, nil
}

func parseRuleID(args []string) (int64, error) {
	if len(args) != 1 {
		return 0, errors.New("usage: /unsubscribe <rule_id>")
	}
	ruleID, err := strconv.ParseInt(strings.TrimSpace(args[0]), 10, 64)
	if err != nil || ruleID <= 0 {
		return 0, errors.New("rule_id must be a positive number")
	}
	return ruleID, nil
}

func parseEventTypes(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "*" {
		return DefaultEventTypes(), nil
	}

	allowed := make(map[string]struct{}, len(allEventTypes))
	for _, eventType := range allEventTypes {
		allowed[eventType] = struct{}{}
	}

	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		eventType := strings.ToLower(strings.TrimSpace(part))
		if eventType == "" {
			continue
		}
		if _, ok := allowed[eventType]; !ok {
			return nil, fmt.Errorf("unknown event_type %q", eventType)
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		out = append(out, eventType)
	}
	if len(out) == 0 {
		return nil, errors.New("event_types must include at least one event")
	}
	return out, nil
}

func normalizeFilter(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "*" {
		return ""
	}
	return value
}

func normalizeExchangeFilter(value string) string {
	normalized := normalizeFilter(value)
	if normalized == "" {
		return ""
	}
	return exchangeinfo.NormalizeSlug(normalized)
}

func DefaultEventTypes() []string {
	out := make([]string, len(allEventTypes))
	copy(out, allEventTypes)
	return out
}

func AvailabilityEventTypes() []string {
	out := make([]string, len(availabilityEventTypes))
	copy(out, availabilityEventTypes)
	return out
}

func IsWildcard(value string) bool {
	return strings.TrimSpace(value) == ""
}
