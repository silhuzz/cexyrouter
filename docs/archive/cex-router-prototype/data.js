// Mock data based on cex-router-api shape: rails (exchange × asset × chain) with health, fees, limits.
window.RAILDEX_DATA = (function () {
  const NOW = Date.now();
  const m = (min) => NOW - min * 60 * 1000;

  const exchanges = [
    {
      id: 1, code: "BNB", name: "BINANCE", region: "GLOBAL",
      tier: "S", color: "#f0b90b",
      tagline: "Largest spot volume. Wide chain support.",
      uptime: 99.6, latency: 142, withdrawals: "OPEN", freshness: m(0.3),
      stats: { vol24h: "27.4B", pairs: 1480, chains: 64, fiat: 14 },
    },
    {
      id: 2, code: "CBS", name: "COINBASE", region: "US",
      tier: "A", color: "#1652f0",
      tagline: "US-regulated. Premium UX, fewer chains.",
      uptime: 99.9, latency: 188, withdrawals: "OPEN", freshness: m(0.5),
      stats: { vol24h: "2.1B", pairs: 312, chains: 18, fiat: 4 },
    },
    {
      id: 3, code: "KRK", name: "KRAKEN", region: "GLOBAL",
      tier: "A", color: "#5848d6",
      tagline: "Old guard. Strong fiat rails, solid uptime.",
      uptime: 99.8, latency: 165, withdrawals: "OPEN", freshness: m(0.4),
      stats: { vol24h: "1.6B", pairs: 410, chains: 22, fiat: 8 },
    },
    {
      id: 4, code: "OKX", name: "OKX", region: "GLOBAL",
      tier: "S", color: "#000000",
      tagline: "Deep liquidity on alt chains.",
      uptime: 99.4, latency: 158, withdrawals: "OPEN", freshness: m(0.8),
      stats: { vol24h: "5.8B", pairs: 720, chains: 51, fiat: 7 },
    },
    {
      id: 5, code: "BYB", name: "BYBIT", region: "GLOBAL",
      tier: "S", color: "#f7a600",
      tagline: "Derivatives focus, fast withdrawals.",
      uptime: 99.2, latency: 134, withdrawals: "OPEN", freshness: m(0.2),
      stats: { vol24h: "8.9B", pairs: 590, chains: 47, fiat: 6 },
    },
    {
      id: 6, code: "BFX", name: "BITFINEX", region: "GLOBAL",
      tier: "B", color: "#16b157",
      tagline: "Veteran venue. Tether home base.",
      uptime: 99.3, latency: 210, withdrawals: "OPEN", freshness: m(1.4),
      stats: { vol24h: "640M", pairs: 280, chains: 26, fiat: 3 },
    },
    {
      id: 7, code: "BST", name: "BITSTAMP", region: "EU",
      tier: "B", color: "#26b1e6",
      tagline: "EU compliance, conservative listings.",
      uptime: 99.7, latency: 196, withdrawals: "DEGRADED", freshness: m(2.1),
      stats: { vol24h: "180M", pairs: 95, chains: 12, fiat: 5 },
    },
    {
      id: 8, code: "KUC", name: "KUCOIN", region: "GLOBAL",
      tier: "A", color: "#24ae8f",
      tagline: "Long tail listings.",
      uptime: 98.4, latency: 244, withdrawals: "OPEN", freshness: m(0.7),
      stats: { vol24h: "1.1B", pairs: 1200, chains: 58, fiat: 0 },
    },
    {
      id: 9, code: "GIO", name: "GATE.IO", region: "GLOBAL",
      tier: "A", color: "#2354e6",
      tagline: "Massive chain coverage, variable fees.",
      uptime: 98.1, latency: 220, withdrawals: "OPEN", freshness: m(1.1),
      stats: { vol24h: "1.9B", pairs: 1650, chains: 78, fiat: 0 },
    },
    {
      id: 10, code: "MXC", name: "MEXC", region: "GLOBAL",
      tier: "B", color: "#2f7df5",
      tagline: "New listings first. Volatile health.",
      uptime: 97.8, latency: 270, withdrawals: "OPEN", freshness: m(1.6),
      stats: { vol24h: "1.3B", pairs: 2100, chains: 70, fiat: 0 },
    },
    {
      id: 11, code: "HTX", name: "HTX", region: "GLOBAL",
      tier: "B", color: "#1c64f2",
      tagline: "Legacy Huobi rails.",
      uptime: 97.1, latency: 295, withdrawals: "OFFLINE", freshness: m(8.4),
      stats: { vol24h: "780M", pairs: 540, chains: 44, fiat: 1 },
    },
    {
      id: 12, code: "BTU", name: "BITGET", region: "GLOBAL",
      tier: "A", color: "#54ffd2",
      tagline: "Copy-trading focus, growing rails.",
      uptime: 99.0, latency: 176, withdrawals: "OPEN", freshness: m(0.6),
      stats: { vol24h: "2.4B", pairs: 700, chains: 40, fiat: 4 },
    },
  ];

  const chains = [
    { id: "ETH",  name: "ETHEREUM",  finality: 78,  block: 12 },
    { id: "ARB",  name: "ARBITRUM",  finality: 4,   block: 0.25 },
    { id: "OP",   name: "OPTIMISM",  finality: 4,   block: 2 },
    { id: "BASE", name: "BASE",      finality: 4,   block: 2 },
    { id: "POL",  name: "POLYGON",   finality: 64,  block: 2 },
    { id: "BSC",  name: "BNB CHAIN", finality: 45,  block: 3 },
    { id: "SOL",  name: "SOLANA",    finality: 13,  block: 0.4 },
    { id: "AVAX", name: "AVALANCHE", finality: 3,   block: 2 },
    { id: "TRX",  name: "TRON",      finality: 57,  block: 3 },
    { id: "BTC",  name: "BITCOIN",   finality: 60,  block: 600 },
    { id: "NEAR", name: "NEAR",      finality: 2,   block: 1 },
    { id: "TON",  name: "TON",       finality: 8,   block: 5 },
  ];

  const coins = [
    { sym: "USDC", name: "USD COIN",     kind: "STABLE", decimals: 6 },
    { sym: "USDT", name: "TETHER",       kind: "STABLE", decimals: 6 },
    { sym: "ETH",  name: "ETHER",        kind: "L1",     decimals: 18 },
    { sym: "BTC",  name: "BITCOIN",      kind: "L1",     decimals: 8 },
    { sym: "SOL",  name: "SOLANA",       kind: "L1",     decimals: 9 },
    { sym: "ARB",  name: "ARBITRUM",     kind: "GOV",    decimals: 18 },
    { sym: "OP",   name: "OPTIMISM",     kind: "GOV",    decimals: 18 },
    { sym: "AVAX", name: "AVALANCHE",    kind: "L1",     decimals: 18 },
    { sym: "MATIC", name: "POLYGON",     kind: "L1",     decimals: 18 },
    { sym: "NEAR", name: "NEAR",         kind: "L1",     decimals: 24 },
  ];

  // Map: which exchanges support each (coin, chain) deposit/withdraw rail.
  // Pseudo-random but deterministic per pair so it's stable across reloads.
  const hash = (s) => { let h = 0; for (const c of s) h = (h * 31 + c.charCodeAt(0)) | 0; return Math.abs(h); };
  // Realistic flat withdrawal fees in the asset being withdrawn.
  // Per-pair base fee × per-exchange jitter. Ethereum mainnet is expensive
  // due to gas; L2s, Solana, Tron etc. are cheap. Native asset on native chain is small.
  const flatFeeBase = {
    USDC:  { ETH: 20,    ARB: 1,     OP: 1,     BASE: 1,    POL: 0.5,  BSC: 0.5, SOL: 1,    AVAX: 0.5, TRX: 1,    NEAR: 0.5, TON: 0.5 },
    USDT:  { ETH: 25,    ARB: 1,     OP: 1,     BASE: 1,    POL: 0.5,  BSC: 0.5, SOL: 1,    AVAX: 0.5, TRX: 1,    NEAR: 0.5, TON: 0.5 },
    ETH:   { ETH: 0.005, ARB: 0.0002, OP: 0.0002, BASE: 0.0002 },
    BTC:   { BTC: 0.0003 },
    SOL:   { SOL: 0.005 },
    ARB:   { ARB: 0.3,   ETH: 3 },
    OP:    { OP: 0.3,    ETH: 3 },
    AVAX:  { AVAX: 0.01 },
    MATIC: { POL: 0.2,   ETH: 8 },
    NEAR:  { NEAR: 0.01 },
  };

  const supportRails = {};
  for (const c of coins) {
    for (const ch of chains) {
      const list = [];
      for (const ex of exchanges) {
        const h = hash(ex.code + c.sym + ch.id);
        // Stablecoins on common L1/L2s: high coverage. BTC only on Bitcoin chain. ETH on EVMs.
        let supports = false;
        if (c.kind === "STABLE") supports = (h % 10) < 8 && !["BTC"].includes(ch.id);
        else if (c.sym === "BTC") supports = ch.id === "BTC" && (h % 10) < 9;
        else if (c.sym === "ETH") supports = ["ETH","ARB","OP","BASE"].includes(ch.id) && (h % 10) < 8;
        else if (c.sym === "SOL") supports = ch.id === "SOL" && (h % 10) < 9;
        else if (c.sym === "AVAX") supports = ch.id === "AVAX" && (h % 10) < 9;
        else if (c.sym === "MATIC") supports = ["POL","ETH"].includes(ch.id) && (h % 10) < 8;
        else if (c.sym === "ARB") supports = ["ARB","ETH"].includes(ch.id) && (h % 10) < 7;
        else if (c.sym === "OP") supports = ["OP","ETH"].includes(ch.id) && (h % 10) < 7;
        else if (c.sym === "NEAR") supports = ch.id === "NEAR" && (h % 10) < 8;
        if (supports) {
          const base = (flatFeeBase[c.sym] && flatFeeBase[c.sym][ch.id]) || 0.5;
          // jitter ±25% per exchange so different venues quote different fees
          const jitter = 0.75 + (h % 50) / 100;
          const rawFee = base * jitter;
          // round to a sensible precision based on magnitude
          let feeFlat;
          if (rawFee >= 10)       feeFlat = Math.round(rawFee);
          else if (rawFee >= 1)   feeFlat = +rawFee.toFixed(2);
          else if (rawFee >= 0.01) feeFlat = +rawFee.toFixed(3);
          else                    feeFlat = +rawFee.toFixed(5);
          const minWdRaw = feeFlat * (2 + (h % 5));
          const minPrecision = c.sym === "BTC" ? 5 : c.sym === "ETH" ? 4 : (feeFlat < 0.01 ? 4 : feeFlat < 1 ? 3 : 2);
          const minWd = +minWdRaw.toFixed(minPrecision);
          const maxWd = c.sym === "BTC" ? 100 : c.sym === "ETH" ? 5000 : c.kind === "STABLE" ? 1000000 : 100000;
          const status = ex.withdrawals === "OFFLINE" ? "OFFLINE"
                        : ex.withdrawals === "DEGRADED" && (h % 4 === 0) ? "DEGRADED"
                        : (h % 17 === 0) ? "DEGRADED"
                        : "OPEN";
          list.push({
            exchange: ex.code,
            feeFlat,
            feeCoin: c.sym,
            minWd,
            maxWd,
            eta: 30 + (h % 600),
            status,
          });
        }
      }
      supportRails[`${c.sym}/${ch.id}`] = list;
    }
  }

  const feedSeed = [
    { t: m(0.05), sev: "ok",   ex: "BYB", msg: "WITHDRAWALS RESUMED · USDC ON SOL" },
    { t: m(0.4),  sev: "warn", ex: "HTX", msg: "RAIL OFFLINE · ALL CHAINS" },
    { t: m(0.9),  sev: "info", ex: "BNB", msg: "FEE DROP · USDT/TRX 1.0 → 0.6 USDT" },
    { t: m(1.5),  sev: "ok",   ex: "OKX", msg: "ETH/BASE QUEUE CLEARED" },
    { t: m(2.2),  sev: "warn", ex: "BST", msg: "DEGRADED · BTC WITHDRAWALS DELAYED 22m" },
    { t: m(3.4),  sev: "info", ex: "CBS", msg: "NEW RAIL · ETH ON BASE LISTED" },
    { t: m(5.1),  sev: "ok",   ex: "GIO", msg: "MAINTENANCE COMPLETE · SOL RESUMED" },
    { t: m(7.6),  sev: "warn", ex: "MXC", msg: "LATENCY SPIKE · 412ms AVG" },
    { t: m(11.2), sev: "info", ex: "KRK", msg: "FEE UPDATE · ETH/L1 GAS PASSTHRU" },
    { t: m(14.0), sev: "ok",   ex: "BTU", msg: "ARB DEPOSITS BACK ONLINE" },
    { t: m(18.8), sev: "info", ex: "KUC", msg: "NEW LISTING · DOGE/SOL" },
    { t: m(24.0), sev: "warn", ex: "BFX", msg: "MAINTENANCE WINDOW · 30m" },
  ];

  // Flatten supportRails into a per-rail list: one row per (exchange, coin, chain).
  // Each row carries deposit + withdraw status, fee, min/max, eta, freshness.
  const allRails = [];
  let railId = 1;
  for (const c of coins) {
    for (const ch of chains) {
      const list = supportRails[`${c.sym}/${ch.id}`] || [];
      for (const r of list) {
        const ex = exchanges.find(e => e.code === r.exchange);
        if (!ex) continue;
        // freshness: combine exchange freshness with a small per-rail offset
        const offset = (hash(c.sym + ch.id + r.exchange) % 240) * 1000;
        allRails.push({
          id: railId++,
          coin: c.sym,
          coinName: c.name,
          chain: ch.id,
          chainName: ch.name,
          exchange: r.exchange,
          exchangeName: ex.name,
          tier: ex.tier,
          region: ex.region,
          feeFlat: r.feeFlat,
          feeCoin: r.feeCoin,
          minWd: r.minWd,
          maxWd: r.maxWd,
          eta: r.eta,
          status: r.status,
          freshness: ex.freshness - offset,
          deposit: r.status === "OFFLINE" ? "OFFLINE" : "OPEN",
          withdraw: r.status,
          finality: ch.finality,
        });
      }
    }
  }
  // Stable sort: status (OPEN first), then exchange, then coin, then chain
  const statusRank = { OPEN: 0, DEGRADED: 1, OFFLINE: 2 };
  allRails.sort((a, b) => {
    if (statusRank[a.status] !== statusRank[b.status]) return statusRank[a.status] - statusRank[b.status];
    if (a.exchange !== b.exchange) return a.exchange.localeCompare(b.exchange);
    if (a.coin !== b.coin) return a.coin.localeCompare(b.coin);
    return a.chain.localeCompare(b.chain);
  });

  return { exchanges, chains, coins, supportRails, feedSeed, allRails };
})();
