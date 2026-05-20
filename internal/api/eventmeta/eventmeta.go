package eventmeta

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/shopspring/decimal"
)

type Change struct {
	Field        string `json:"field"`
	Label        string `json:"label"`
	Before       string `json:"before"`
	After        string `json:"after"`
	Delta        string `json:"delta,omitempty"`
	DeltaPercent string `json:"delta_percent,omitempty"`
	Direction    string `json:"direction,omitempty"`
}

type Details struct {
	Summary string   `json:"summary"`
	Changes []Change `json:"changes,omitempty"`
}

func Build(eventType string, beforeRaw json.RawMessage, afterRaw json.RawMessage) Details {
	before := decodeObject(beforeRaw)
	after := decodeObject(afterRaw)

	changes := changesForEvent(eventType, before, after)
	if len(changes) == 0 {
		changes = detectChanges(before, after)
	}

	return Details{
		Summary: summarize(eventType, changes),
		Changes: changes,
	}
}

func changesForEvent(eventType string, before map[string]any, after map[string]any) []Change {
	switch eventType {
	case "deposit_off", "deposit_on":
		return compactChanges(statusChange("deposit_enabled", "Deposit", before, after))
	case "withdraw_off", "withdraw_on":
		return compactChanges(statusChange("withdraw_enabled", "Withdraw", before, after))
	case "fee_changed":
		return compactChanges(
			decimalChange("withdraw_fee", "Withdraw fee", before, after),
			decimalChange("withdraw_fee_percent", "Fee percent", before, after),
			textChange("withdraw_fee_type", "Fee type", before, after),
		)
	case "min_changed":
		return compactChanges(decimalChange("withdraw_min", "Withdraw min", before, after))
	case "rail_delisted", "rail_relisted":
		return compactChanges(statusChange("is_active", "Rail active", before, after))
	default:
		return nil
	}
}

func detectChanges(before map[string]any, after map[string]any) []Change {
	return compactChanges(
		statusChange("deposit_enabled", "Deposit", before, after),
		statusChange("withdraw_enabled", "Withdraw", before, after),
		decimalChange("withdraw_fee", "Withdraw fee", before, after),
		decimalChange("withdraw_fee_percent", "Fee percent", before, after),
		textChange("withdraw_fee_type", "Fee type", before, after),
		decimalChange("withdraw_min", "Withdraw min", before, after),
		textChange("deposit_confirmations", "Confirmations", before, after),
		statusChange("is_active", "Rail active", before, after),
		textChange("missing_count", "Missing count", before, after),
	)
}

func statusChange(field string, label string, before map[string]any, after map[string]any) *Change {
	beforeValue, beforeOK := boolValue(before, field)
	afterValue, afterOK := boolValue(after, field)
	if !beforeOK || !afterOK || beforeValue == afterValue {
		return nil
	}
	return &Change{
		Field:     field,
		Label:     label,
		Before:    onOff(beforeValue),
		After:     onOff(afterValue),
		Direction: boolDirection(afterValue),
	}
}

func decimalChange(field string, label string, before map[string]any, after map[string]any) *Change {
	beforeValue, beforeOK := decimalValue(before, field)
	afterValue, afterOK := decimalValue(after, field)
	if !beforeOK || !afterOK || beforeValue.Equal(afterValue) {
		return nil
	}

	delta := afterValue.Sub(beforeValue)
	change := Change{
		Field:     field,
		Label:     label,
		Before:    formatDecimal(beforeValue),
		After:     formatDecimal(afterValue),
		Delta:     formatSignedDecimal(delta),
		Direction: decimalDirection(delta),
	}
	if !beforeValue.IsZero() {
		percent := delta.Div(beforeValue).Mul(decimal.NewFromInt(100))
		if percent.Abs().GreaterThan(decimal.RequireFromString("0.0001")) {
			change.DeltaPercent = formatSignedPercent(percent) + "%"
		}
	}
	return &change
}

func textChange(field string, label string, before map[string]any, after map[string]any) *Change {
	beforeValue, beforeOK := stringValue(before, field)
	afterValue, afterOK := stringValue(after, field)
	if !beforeOK || !afterOK || beforeValue == afterValue {
		return nil
	}
	return &Change{
		Field:  field,
		Label:  label,
		Before: displayValue(beforeValue),
		After:  displayValue(afterValue),
	}
}

func summarize(eventType string, changes []Change) string {
	if len(changes) == 0 {
		return humanize(eventType)
	}

	first := changes[0]
	summary := fmt.Sprintf("%s %s -> %s", first.Label, first.Before, first.After)
	deltaParts := make([]string, 0, 2)
	if first.Delta != "" {
		deltaParts = append(deltaParts, first.Delta)
	}
	if first.DeltaPercent != "" {
		deltaParts = append(deltaParts, first.DeltaPercent)
	}
	if len(deltaParts) > 0 {
		summary += " (" + strings.Join(deltaParts, ", ") + ")"
	}
	if len(changes) > 1 {
		summary += fmt.Sprintf(" + %d more", len(changes)-1)
	}
	return summary
}

func decodeObject(raw json.RawMessage) map[string]any {
	out := make(map[string]any)
	if len(raw) == 0 {
		return out
	}
	_ = json.Unmarshal(raw, &out)
	return out
}

func compactChanges(changes ...*Change) []Change {
	out := make([]Change, 0, len(changes))
	for _, change := range changes {
		if change != nil {
			out = append(out, *change)
		}
	}
	return out
}

func boolValue(obj map[string]any, field string) (bool, bool) {
	value, ok := rawValue(obj, field)
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on", "enabled", "active":
			return true, true
		case "false", "0", "no", "off", "disabled", "inactive":
			return false, true
		}
	case float64:
		return typed != 0, true
	}
	return false, false
}

func decimalValue(obj map[string]any, field string) (decimal.Decimal, bool) {
	value, ok := rawValue(obj, field)
	if !ok || value == nil {
		return decimal.Decimal{}, false
	}
	switch typed := value.(type) {
	case string:
		parsed, err := decimal.NewFromString(strings.TrimSpace(typed))
		return parsed, err == nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return decimal.Decimal{}, false
		}
		return decimal.NewFromFloat(typed), true
	case int:
		return decimal.NewFromInt(int64(typed)), true
	case json.Number:
		parsed, err := decimal.NewFromString(typed.String())
		return parsed, err == nil
	}
	return decimal.Decimal{}, false
}

func stringValue(obj map[string]any, field string) (string, bool) {
	value, ok := rawValue(obj, field)
	if !ok || value == nil {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case bool:
		return onOff(typed), true
	case float64:
		return formatDecimal(decimal.NewFromFloat(typed)), true
	case json.Number:
		return typed.String(), true
	default:
		return fmt.Sprint(typed), true
	}
}

func rawValue(obj map[string]any, field string) (any, bool) {
	for _, key := range keyVariants(field) {
		value, ok := obj[key]
		if ok {
			return value, true
		}
	}
	return nil, false
}

func keyVariants(field string) []string {
	parts := strings.Split(field, "_")
	if len(parts) == 1 {
		return []string{field, upperFirst(field)}
	}
	pascal := strings.Builder{}
	camel := strings.Builder{}
	for index, part := range parts {
		if part == "" {
			continue
		}
		word := upperFirst(part)
		pascal.WriteString(word)
		if index == 0 {
			camel.WriteString(part)
		} else {
			camel.WriteString(word)
		}
	}
	return []string{field, camel.String(), pascal.String()}
}

func upperFirst(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func boolDirection(value bool) string {
	if value {
		return "up"
	}
	return "down"
}

func decimalDirection(delta decimal.Decimal) string {
	if delta.IsPositive() {
		return "up"
	}
	if delta.IsNegative() {
		return "down"
	}
	return ""
}

func formatSignedDecimal(value decimal.Decimal) string {
	if value.IsPositive() {
		return "+" + formatDecimal(value)
	}
	return formatDecimal(value)
}

func formatSignedPercent(value decimal.Decimal) string {
	if value.IsPositive() {
		return "+" + formatPercent(value)
	}
	return formatPercent(value)
}

func formatDecimal(value decimal.Decimal) string {
	out := value.StringFixedBank(8)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "-0" || out == "" {
		return "0"
	}
	return out
}

func formatPercent(value decimal.Decimal) string {
	out := value.StringFixedBank(2)
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "-0" || out == "" {
		return "0"
	}
	return out
}

func displayValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "n/a"
	}
	return value
}

func humanize(value string) string {
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.TrimSpace(value)
	if value == "" {
		return "Rail event"
	}
	return upperFirst(value)
}
