package commands

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type User struct {
	ID       int32
	TGChatID int64
}

type Rule struct {
	ID         int64
	Exchange   string
	Coin       string
	Chain      string
	EventTypes []string
	CreatedAt  time.Time
}

type RailStatus struct {
	Exchange        string
	Coin            string
	Chain           string
	DepositEnabled  bool
	WithdrawEnabled bool
	IsActive        bool
	IsInitial       bool
	WithdrawMin     string
	WithdrawFee     string
	LastSeenAt      time.Time
}

type UnknownReferenceError struct {
	Kind  string
	Value string
}

func (e UnknownReferenceError) Error() string {
	return fmt.Sprintf("unknown %s %q", e.Kind, e.Value)
}

func formatRule(rule Rule) string {
	return fmt.Sprintf("#%d %s/%s/%s events=%s",
		rule.ID,
		displayWildcard(rule.Exchange),
		displayWildcard(rule.Coin),
		displayWildcard(rule.Chain),
		strings.Join(rule.EventTypes, ","),
	)
}

func formatRulesSummary(rules []Rule) string {
	if len(rules) == 0 {
		return "No alert filters yet."
	}

	groups := groupRules(rules)
	lines := []string{
		"Your alert filters",
		"",
		fmt.Sprintf("%d saved %s", len(rules), plural(len(rules), "filter")),
		"Events: " + summarizeRuleEvents(rules),
		"",
		"Coverage:",
	}
	for _, group := range groups {
		lines = append(lines, group.Exchange)
		for _, coin := range group.Coins {
			lines = append(lines, fmt.Sprintf("- %s: %s", coin.Coin, strings.Join(coin.Chains, ", ")))
		}
		lines = append(lines, "")
	}
	lines = append(lines, "Use Reset alert settings to clear everything and stop queued unsent alerts.")
	return strings.Join(lines, "\n")
}

func formatRailStatus(rail RailStatus) string {
	deposit := "deposit off"
	if rail.DepositEnabled {
		deposit = "deposit on"
	}
	withdraw := "withdraw off"
	if rail.WithdrawEnabled {
		withdraw = "withdraw on"
	}
	active := "active"
	if !rail.IsActive {
		active = "delisted"
	} else if rail.IsInitial {
		active = "initializing"
	}

	parts := []string{
		fmt.Sprintf("%s %s/%s", rail.Exchange, rail.Coin, rail.Chain),
		deposit,
		withdraw,
		active,
	}
	if rail.WithdrawFee != "" {
		parts = append(parts, "fee "+rail.WithdrawFee)
	}
	if rail.WithdrawMin != "" {
		parts = append(parts, "min "+rail.WithdrawMin)
	}
	return strings.Join(parts, " | ")
}

func displayWildcard(value string) string {
	if IsWildcard(value) {
		return "*"
	}
	return value
}

type ruleGroup struct {
	Exchange string
	Coins    []ruleCoinGroup
}

type ruleCoinGroup struct {
	Coin   string
	Chains []string
}

func groupRules(rules []Rule) []ruleGroup {
	groups := make([]ruleGroup, 0)
	exchangeIndex := map[string]int{}
	coinIndex := map[string]map[string]int{}
	seenChains := map[string]map[string]map[string]bool{}

	for _, rule := range rules {
		exchange := displayExchange(rule.Exchange)
		coin := displayCoin(rule.Coin)
		chain := displayChain(rule.Chain)

		exIdx, ok := exchangeIndex[exchange]
		if !ok {
			exIdx = len(groups)
			exchangeIndex[exchange] = exIdx
			groups = append(groups, ruleGroup{Exchange: exchange})
			coinIndex[exchange] = map[string]int{}
			seenChains[exchange] = map[string]map[string]bool{}
		}

		cIdx, ok := coinIndex[exchange][coin]
		if !ok {
			cIdx = len(groups[exIdx].Coins)
			coinIndex[exchange][coin] = cIdx
			groups[exIdx].Coins = append(groups[exIdx].Coins, ruleCoinGroup{Coin: coin})
			seenChains[exchange][coin] = map[string]bool{}
		}

		if seenChains[exchange][coin][chain] {
			continue
		}
		seenChains[exchange][coin][chain] = true
		groups[exIdx].Coins[cIdx].Chains = append(groups[exIdx].Coins[cIdx].Chains, chain)
	}
	return groups
}

func summarizeRuleEvents(rules []Rule) string {
	if len(rules) == 0 {
		return "none"
	}
	first := eventKey(rules[0].EventTypes)
	for _, rule := range rules[1:] {
		if eventKey(rule.EventTypes) != first {
			return "mixed alert types"
		}
	}
	switch first {
	case eventKey(AvailabilityEventTypes()):
		return "route availability changes"
	case eventKey(DefaultEventTypes()):
		return "all rail changes, including fee/minimum updates"
	default:
		return strings.Join(rules[0].EventTypes, ", ")
	}
}

func eventKey(events []string) string {
	normalized := append([]string(nil), events...)
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
}

func displayExchange(value string) string {
	if IsWildcard(value) {
		return "All exchanges"
	}
	return displayChoice(value, demoExchangeOptions, strings.ToUpper(value))
}

func displayCoin(value string) string {
	if IsWildcard(value) {
		return "Every token"
	}
	return strings.ToUpper(value)
}

func displayChain(value string) string {
	if IsWildcard(value) {
		return "every chain"
	}
	return displayChoice(value, demoChainOptions, prettySlug(value))
}

func displayChoice(value string, options []demoChoice, fallback string) string {
	for _, option := range options {
		if strings.EqualFold(value, option.Slug) {
			return option.Label
		}
	}
	return fallback
}

func prettySlug(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	known := map[string]string{
		"bsc":  "BSC",
		"trx":  "TRON",
		"tron": "TRON",
	}
	if label, ok := known[strings.ToLower(value)]; ok {
		return label
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}
