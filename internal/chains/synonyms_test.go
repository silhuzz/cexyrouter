package chains

import "testing"

func TestCanonicalSlugDemoSynonyms(t *testing.T) {
	tests := []struct {
		name      string
		rawSymbol string
		rawName   string
		want      string
	}{
		{name: "erc20", rawSymbol: "ERC20", want: "ethereum"},
		{name: "trc20", rawSymbol: "TRC20", want: "tron"},
		{name: "bep20", rawSymbol: "BEP20", want: "bsc"},
		{name: "matic", rawSymbol: "MATIC", want: "polygon"},
		{name: "arbitrum symbol", rawSymbol: "Arbitrum", want: "arbitrum"},
		{name: "arbitrum name", rawName: "Arbitrum One", want: "arbitrum"},
		{name: "optimism", rawSymbol: "Optimism", want: "optimism"},
		{name: "base", rawSymbol: "Base", want: "base"},
		{name: "base evm", rawSymbol: "BASEEVM", want: "base"},
		{name: "arbitrum evm", rawSymbol: "ARBEVM", want: "arbitrum"},
		{name: "arbitrum one compact", rawSymbol: "ArbitrumOne", want: "arbitrum"},
		{name: "polygon pos", rawSymbol: "Polygon POS", want: "polygon"},
		{name: "op eth", rawSymbol: "OPETH", want: "optimism"},
		{name: "linea eth", rawSymbol: "LINEAETH", want: "linea"},
		{name: "avalanche c chain", rawSymbol: "AVAXC-Chain", want: "avax"},
		{name: "avalanche c chain name", rawName: "Avalanche C-Chain", want: "avax"},
		{name: "coinex avalanche c", rawSymbol: "AVA_C", want: "avax"},
		{name: "htx avalanche c", rawSymbol: "AVAXCCHAIN", want: "avax"},
		{name: "bitmart bsc bnb", rawSymbol: "BSC_BNB", want: "bsc"},
		{name: "bitmart base eth", rawSymbol: "BASE-ETH", want: "base"},
		{name: "aptos", rawSymbol: "APTOS", want: "aptos"},
		{name: "zk sync era name", rawName: "zkSync Era", want: "zksyncera"},
		{name: "zk sync era compact canonical", rawSymbol: "zkSyncEra", want: "zksyncera"},
		{name: "zk sync era compact", rawSymbol: "ZKSERA", want: "zksyncera"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CanonicalSlug(tt.rawSymbol, tt.rawName)
			if !ok {
				t.Fatalf("CanonicalSlug(%q, %q) did not resolve", tt.rawSymbol, tt.rawName)
			}
			if got != tt.want {
				t.Fatalf("CanonicalSlug(%q, %q) = %q, want %q", tt.rawSymbol, tt.rawName, got, tt.want)
			}
		})
	}
}
