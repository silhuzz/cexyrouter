package chains

import "strings"

var Synonyms = map[string]string{
	"ARB":                 "arbitrum",
	"ARBEVM":              "arbitrum",
	"ARBITRUM":            "arbitrum",
	"ARBITRUM ONE":        "arbitrum",
	"ARBITRUMONE":         "arbitrum",
	"APT":                 "aptos",
	"APTOS":               "aptos",
	"AVA C":               "avax",
	"AVALANCHE C CHAIN":   "avax",
	"AVA_C":               "avax",
	"AVAX C":              "avax",
	"AVAX C CHAIN":        "avax",
	"AVAXC CHAIN":         "avax",
	"AVAX CCHAIN":         "avax",
	"AVAXCCHAIN":          "avax",
	"BASE":                "base",
	"BASE ETH":            "base",
	"BASEEVM":             "base",
	"BEP20":               "bsc",
	"BEP-20":              "bsc",
	"BINANCE SMART CHAIN": "bsc",
	"BSC":                 "bsc",
	"BSC BNB":             "bsc",
	"BTC":                 "bitcoin",
	"BITCOIN":             "bitcoin",
	"CARDANO":             "cardano",
	"DOT":                 "polkadot",
	"ERC20":               "ethereum",
	"ERC-20":              "ethereum",
	"ETH":                 "ethereum",
	"ETHEREUM":            "ethereum",
	"ETHEREUM NETWORK":    "ethereum",
	"LINEAETH":            "linea",
	"MATIC":               "polygon",
	"OPTIMISM":            "optimism",
	"OP":                  "optimism",
	"OPETH":               "optimism",
	"POLKADOT":            "polkadot",
	"POLYGON POS":         "polygon",
	"POLYGON":             "polygon",
	"BLASTETH":            "blast",
	"SOL":                 "solana",
	"SOLANA":              "solana",
	"TRC20":               "tron",
	"TRC-20":              "tron",
	"TRON":                "tron",
	"XRP":                 "ripple",
	"XRP LEDGER":          "ripple",
	"ZKSYNC ERA":          "zksyncera",
	"ZKSYNCERA":           "zksyncera",
	"ZKSERA":              "zksyncera",
}

func CanonicalSlug(rawSymbol string, rawName string) (string, bool) {
	candidates := []string{rawSymbol, rawName}
	for _, candidate := range candidates {
		normalized := normalize(candidate)
		if normalized == "" {
			continue
		}
		if slug, ok := Synonyms[normalized]; ok {
			return slug, true
		}
	}
	return "", false
}

func normalize(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	return strings.ToUpper(value)
}
