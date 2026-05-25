package exchanges

import "strings"

type Venue struct {
	Slug string
	Name string
}

const MainVenueSlugsCSV = "binance,bybit,bitget,gate"

var MainVenues = []Venue{
	{Slug: "binance", Name: "Binance"},
	{Slug: "bybit", Name: "Bybit"},
	{Slug: "bitget", Name: "Bitget"},
	{Slug: "gate", Name: "Gate.io"},
}

func NormalizeSlug(raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	if slug == "" {
		return ""
	}

	compact := strings.NewReplacer(" ", "", "_", "", "-", "").Replace(slug)
	switch compact {
	case "gate.io", "gateio":
		return "gate"
	default:
		return slug
	}
}

func NormalizeSlugs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		slug := NormalizeSlug(value)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	return out
}
