(() => {
  const ENDPOINTS = {
    exchanges: "/v1/exchanges",
    coins: "/v1/coins",
    chains: "/v1/chains",
    rails: "/v1/rails",
    routeOptions: "/v1/route-options",
    routes: "/v1/routes",
    events: "/v1/events",
    freshness: "/v1/adapters/freshness",
    ws: "/v1/ws",
  };

  const FRESHNESS_STALE_MS = 10 * 60 * 1000;
  const EVENTS_WINDOW = "24h";
  const AVAILABILITY_EVENT_TYPES = [
    "deposit_off",
    "deposit_on",
    "withdraw_off",
    "withdraw_on",
    "rail_delisted",
    "rail_relisted",
  ];

  const EXCHANGES = [
    { slug: "okx", name: "OKX" },
    { slug: "bithumb", name: "Bithumb" },
    { slug: "bitget", name: "Bitget" },
    { slug: "kucoin", name: "KuCoin" },
    { slug: "gate", name: "Gate" },
    { slug: "htx", name: "HTX" },
    { slug: "coinex", name: "CoinEx" },
    { slug: "whitebit", name: "WhiteBIT" },
    { slug: "bitmart", name: "BitMart" },
  ];
  const INTEGRATED_EXCHANGE_SLUGS = new Set(EXCHANGES.map((exchange) => exchange.slug));

  const FALLBACK_COINS = [
    { slug: "usdt", symbol: "USDT", name: "Tether" },
    { slug: "usdc", symbol: "USDC", name: "USD Coin" },
    { slug: "btc", symbol: "BTC", name: "Bitcoin" },
    { slug: "wbtc", symbol: "WBTC", name: "Wrapped Bitcoin" },
    { slug: "tbtc", symbol: "TBTC", name: "tBTC" },
    { slug: "eth", symbol: "ETH", name: "Ether" },
    { slug: "weth", symbol: "WETH", name: "Wrapped Ether" },
    { slug: "xrp", symbol: "XRP", name: "XRP" },
  ];

  const FALLBACK_CHAINS = [
    { slug: "ethereum", symbol: "ETH", name: "Ethereum" },
    { slug: "tron", symbol: "TRON", name: "Tron" },
    { slug: "bsc", symbol: "BSC", name: "BNB Smart Chain" },
    { slug: "solana", symbol: "SOL", name: "Solana" },
    { slug: "polygon", symbol: "POLYGON", name: "Polygon" },
    { slug: "arbitrum", symbol: "ARB", name: "Arbitrum" },
    { slug: "optimism", symbol: "OP", name: "Optimism" },
    { slug: "base", symbol: "BASE", name: "Base" },
    { slug: "bitcoin", symbol: "BTC", name: "Bitcoin" },
  ];

  const PRIORITY_PAIRS = [
    ["usdt", "tron"],
    ["usdt", "ethereum"],
    ["usdt", "bsc"],
    ["usdt", "solana"],
    ["usdt", "polygon"],
    ["usdc", "ethereum"],
    ["usdc", "solana"],
    ["btc", "bitcoin"],
    ["eth", "ethereum"],
  ];

  const state = {
    exchanges: EXCHANGES,
    coins: FALLBACK_COINS,
    chains: FALLBACK_CHAINS,
    rails: [],
    freshness: [],
    events: [],
    routeOptions: null,
    eventIds: new Set(),
    lastCursor: null,
    eventFilters: {
      exchange: "",
      coin: "",
      chain: "",
      kind: "availability",
    },
    ws: null,
    reconnectTimer: null,
    liveRefreshTimer: null,
    routeResultsVisible: false,
  };

  const $ = (selector) => document.querySelector(selector);

  document.addEventListener("DOMContentLoaded", () => {
    $("#refresh-board").addEventListener("click", () => {
      refreshAll();
    });
    $("#route-form").addEventListener("submit", (event) => {
      event.preventDefault();
      findRoutes();
    });
    $("#coin-select").addEventListener("change", () => {
      refreshRouteOptions();
    });
    $("#equivalent-assets-input").addEventListener("change", () => {
      refreshRouteOptions();
    });
    $("#from-chain-select").addEventListener("change", () => {
      applyRouteOptionsToChainSelects({ preserveFrom: true });
    });
    ["#event-exchange-filter", "#event-coin-filter", "#event-chain-filter", "#event-kind-filter"].forEach((selector) => {
      $(selector).addEventListener("change", () => {
        readEventFilters();
        resetEventsFeed();
        loadInitialEvents();
        connectEventsSocket();
      });
    });

    bootstrap();
  });

  async function bootstrap() {
    await loadReferenceData();
    populateRouteForm();
    populateEventFilters();
    readEventFilters();
    await refreshRouteOptions({ initial: true });
    await Promise.allSettled([refreshAll(), loadInitialEvents()]);
    connectEventsSocket();
  }

  async function refreshAll() {
    await Promise.allSettled([loadRails(), loadFreshness()]);
  }

  async function loadReferenceData() {
    const [exchangeResult, coinResult, chainResult] = await Promise.allSettled([
      getJSON(ENDPOINTS.exchanges),
      getJSON(ENDPOINTS.coins),
      getJSON(ENDPOINTS.chains),
    ]);

    if (exchangeResult.status === "fulfilled") {
      const exchanges = normalizeReference(exchangeResult.value, ["exchanges", "items", "data"], "exchange")
        .filter((exchange) => INTEGRATED_EXCHANGE_SLUGS.has(exchange.slug));
      state.exchanges = mergeReference(
        EXCHANGES,
        exchanges
      );
    }
    if (coinResult.status === "fulfilled") {
      state.coins = mergeReference(
        FALLBACK_COINS,
        normalizeReference(coinResult.value, ["coins", "items", "data"], "coin")
      );
    }
    if (chainResult.status === "fulfilled") {
      state.chains = mergeReference(
        FALLBACK_CHAINS,
        normalizeReference(chainResult.value, ["chains", "items", "data"], "chain")
      );
    }
  }

  async function loadRails() {
    setPill("#api-status", "REST loading rails", "neutral");
    try {
      const payload = await getJSON(`${ENDPOINTS.rails}?limit=500`);
      state.rails = pickArray(payload, ["rails", "items", "data"]);
      renderStatusBoard();
      setPill("#api-status", "REST online", "ok");
    } catch (error) {
      state.rails = [];
      renderStatusBoard(error);
      setPill("#api-status", "REST unavailable", "bad");
    }
  }

  async function loadFreshness() {
    try {
      const payload = await getJSON(ENDPOINTS.freshness);
      state.freshness = pickArray(payload, ["freshness", "adapters", "items", "data"]);
      renderFreshness();
    } catch (error) {
      state.freshness = [];
      renderFreshness(error);
    }
  }

  async function loadInitialEvents() {
    try {
      const payload = await getJSON(`${ENDPOINTS.events}?${eventQueryParams().toString()}`);
      const rows = pickArray(payload, ["events", "items", "data"]);
      rows.slice().reverse().forEach(addEvent);
      renderEvents();
    } catch (error) {
      renderEvents(error);
    }
  }

  async function findRoutes() {
    const button = $("#find-route");
    const results = $("#route-results");
    const coin = $("#coin-select").value;

    button.disabled = true;
    button.textContent = "Finding";
    results.innerHTML = `<p class="muted">Ranking open rails for ${escapeHTML(displayCoin(coin))}</p>`;

    try {
      const routes = await loadRoutes();
      renderRoutes(routes);
      state.routeResultsVisible = true;
    } catch (error) {
      results.innerHTML = `<p class="error-text">${escapeHTML(error.message)}</p>`;
    } finally {
      button.disabled = false;
      button.textContent = "Find routes";
    }
  }

  function connectEventsSocket() {
    clearTimeout(state.reconnectTimer);
    if (state.ws) {
      state.ws.close();
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const socket = new WebSocket(`${protocol}//${window.location.host}${ENDPOINTS.ws}`);
    state.ws = socket;
    setPill("#ws-status", "WS connecting", "neutral");

    socket.addEventListener("open", () => {
      setPill("#ws-status", "WS live", "ok");
      socket.send(JSON.stringify({
        type: "subscribe",
        filters: eventSocketFilters(),
        since: state.lastCursor,
      }));
    });

    socket.addEventListener("message", (message) => {
      const payload = parseJSON(message.data);
      if (!payload || payload.type === "heartbeat" || payload.type === "pong") {
        return;
      }

      const event = unwrapEvent(payload);
      if (!event) {
        return;
      }

      state.lastCursor = field(event, ["cursor"], field(payload, ["cursor"], state.lastCursor));
      addEvent(event);
      renderEvents();
      scheduleLiveRefresh();
    });

    socket.addEventListener("close", () => {
      setPill("#ws-status", "WS reconnecting", "warn");
      state.reconnectTimer = window.setTimeout(connectEventsSocket, 2000);
    });

    socket.addEventListener("error", () => {
      setPill("#ws-status", "WS error", "bad");
    });
  }

  function renderFreshness(error) {
    const banner = $("#freshness-banner");
    const summary = $("#freshness-summary");
    const chips = $("#freshness-chips");

    if (error) {
      banner.className = "freshness-banner bad";
      summary.textContent = `Freshness endpoint unavailable: ${error.message}`;
      chips.innerHTML = "";
      return;
    }

    const rows = state.freshness
      .filter((row) => exchangeSlug(row))
      .sort((a, b) => displayExchange(exchangeSlug(a)).localeCompare(displayExchange(exchangeSlug(b))));
    let failing = 0;
    let stale = 0;
    const html = rows.map((row) => {
      const exchange = displayExchange(exchangeSlug(row));

      const failures = Number(rawField(row, ["consecutive_failures", "consecutiveFailures"], 0)) || 0;
      const lastPoll = field(row, ["last_successful_poll", "lastSuccessfulPoll"], "");
      const ageMs = lastPoll ? Date.now() - new Date(lastPoll).getTime() : Infinity;
      const isStale = !Number.isFinite(ageMs) || ageMs > FRESHNESS_STALE_MS;

      if (failures > 0) {
        failing += 1;
        return `<span class="chip bad">${escapeHTML(exchange)} ${failures} fail${failures === 1 ? "" : "s"}</span>`;
      }
      if (isStale) {
        stale += 1;
        return `<span class="chip warn">${escapeHTML(exchange)} ${escapeHTML(relativeTime(lastPoll))}</span>`;
      }
      return `<span class="chip ok">${escapeHTML(exchange)} ${escapeHTML(relativeTime(lastPoll))}</span>`;
    }).join("");

    chips.innerHTML = html || `<span class="chip neutral">Waiting for adapter polls</span>`;
    if (failing > 0) {
      banner.className = "freshness-banner bad";
      summary.textContent = `${failing} adapter${failing === 1 ? "" : "s"} reporting failures`;
    } else if (stale > 0) {
      banner.className = "freshness-banner warn";
      summary.textContent = `${stale} adapter${stale === 1 ? "" : "s"} missing a recent successful poll`;
    } else {
      banner.className = "freshness-banner ok";
      summary.textContent = "All exchange adapters are fresh";
    }
  }

  function renderStatusBoard(error) {
    const head = $("#status-head");
    const body = $("#status-body");

    if (error) {
      head.innerHTML = "";
      body.innerHTML = `<tr><td class="loading-cell">Rails endpoint unavailable: ${escapeHTML(error.message)}</td></tr>`;
      return;
    }

    const pairs = deriveHotPairs().slice(0, 8);
    head.innerHTML = `<tr><th>Exchange</th>${pairs.map((pair) => `<th>${escapeHTML(pairLabel(pair))}</th>`).join("")}</tr>`;
    body.innerHTML = state.exchanges.map((exchange) => {
      const cells = pairs.map((pair) => renderRailCell(exchange.slug, pair)).join("");
      return `<tr><td class="exchange-cell">${escapeHTML(exchange.name)}</td>${cells}</tr>`;
    }).join("");
  }

  function renderRailCell(exchange, pair) {
    const rail = state.rails.find((row) => {
      return exchangeSlug(row) === exchange && coinSlug(row) === pair.coin && chainSlug(row) === pair.chain;
    });

    if (!rail) {
      return `<td class="status-cell"><div class="status-card missing"><strong>No rail</strong><small>Not listed in current page</small></div></td>`;
    }

    const active = boolField(rail, ["is_active", "isActive"], true);
    const deposit = boolField(rail, ["deposit_enabled", "depositEnabled"], false);
    const withdraw = boolField(rail, ["withdraw_enabled", "withdrawEnabled"], false);
    const fee = field(rail, ["withdraw_fee", "withdrawFee"], "");
    const min = field(rail, ["withdraw_min", "withdrawMin"], "");
    const seen = field(rail, ["last_seen_at", "lastSeenAt"], "");

    let label = "Closed";
    let statusClass = "closed";
    if (!active) {
      label = "Delisted";
      statusClass = "inactive";
    } else if (deposit && withdraw) {
      label = "Open";
      statusClass = "open";
    } else if (deposit || withdraw) {
      label = deposit ? "Deposit only" : "Withdraw only";
      statusClass = "partial";
    }

    const meta = [
      `D:${deposit ? "on" : "off"} W:${withdraw ? "on" : "off"}`,
      fee ? `Fee ${fee}` : "",
      min ? `Min ${min}` : "",
      seen ? relativeTime(seen) : "",
    ].filter(Boolean).join(" | ");

    return `<td class="status-cell"><div class="status-card ${statusClass}"><strong>${label}</strong><small>${escapeHTML(meta)}</small></div></td>`;
  }

  function renderRoutes(routes) {
    const results = $("#route-results");
    if (routes.length === 0) {
      results.innerHTML = `<p class="muted">No open route for this pair yet.</p>`;
      return;
    }

    results.innerHTML = routes.map((route, index) => {
      const depositRail = rawField(route, ["deposit_rail", "depositRail", "from_rail", "fromRail"], {});
      const withdrawRail = rawField(route, ["withdraw_rail", "withdrawRail", "to_rail", "toRail"], {});
      const exchange = field(route, ["exchange_slug", "exchange", "exchange.slug"], field(depositRail, ["exchange_slug", "exchange", "exchange.slug"], ""));
      const fee = field(route, ["total_fee_estimate", "totalFeeEstimate", "total_fee", "totalFee"], "n/a");
      const fromCoin = field(route, ["from_coin", "fromCoin", "coin"], field(depositRail, ["coin_slug", "coinSlug", "coin"], ""));
      const toCoin = field(route, ["to_coin", "toCoin", "coin"], field(withdrawRail, ["coin_slug", "coinSlug", "coin"], ""));
      const fromChain = field(route, ["from_chain", "fromChain"], $("#from-chain-select").value);
      const toChain = field(route, ["to_chain", "toChain"], $("#to-chain-select").value);
      const routeKind = field(route, ["route_kind", "routeKind"], "");
      const equivalentAsset = routeKind === "equivalent_asset" || boolField(route, ["equivalent_asset", "equivalentAsset"], false);
      const depositLast = field(depositRail, ["last_seen_at", "lastSeenAt"], "");
      const withdrawLast = field(withdrawRail, ["last_seen_at", "lastSeenAt"], "");
      const seen = [depositLast, withdrawLast].filter(Boolean).map(relativeTime).join(" / ");

      return `
        <article class="route-item">
          <div class="route-title">
            <strong>#${index + 1} ${escapeHTML(displayExchange(exchange))}</strong>
            ${equivalentAsset ? `<span class="route-badge">Equivalent</span>` : ""}
          </div>
          <div class="route-meta">${escapeHTML(displayCoin(fromCoin))} / ${escapeHTML(displayChain(fromChain))} deposit -> ${escapeHTML(displayCoin(toCoin))} / ${escapeHTML(displayChain(toChain))} withdrawal</div>
          <div class="route-meta">Estimated fee: ${escapeHTML(String(fee))}${seen ? ` | Seen ${escapeHTML(seen)}` : ""}</div>
        </article>
      `;
    }).join("");
  }

  async function loadRoutes() {
    const coin = $("#coin-select").value;
    const fromChain = $("#from-chain-select").value;
    const toChain = $("#to-chain-select").value;
    const amount = $("#amount-input").value.trim();
    const equivalentAssets = $("#equivalent-assets-input").checked;

    const params = new URLSearchParams({
      coin,
      from_chain: fromChain,
      to_chain: toChain,
    });
    if (amount !== "") {
      params.set("amount", amount);
    }
    if (equivalentAssets) {
      params.set("equivalent_assets", "true");
    }

    const payload = await getJSON(`${ENDPOINTS.routes}?${params.toString()}`);
    return pickArray(payload, ["routes", "items", "data"]);
  }

  async function refreshRouteOptions(options = {}) {
    const coin = $("#coin-select").value;
    const equivalentAssets = $("#equivalent-assets-input").checked;
    const params = new URLSearchParams({ coin });
    if (equivalentAssets) {
      params.set("equivalent_assets", "true");
    }

    try {
      const payload = await getJSON(`${ENDPOINTS.routeOptions}?${params.toString()}`);
      state.routeOptions = {
        fromChains: normalizeRouteOptionChains(payload, ["from_chains", "fromChains"]),
        toChains: normalizeRouteOptionChains(payload, ["to_chains", "toChains"]),
        pairs: normalizeRouteOptionPairs(payload),
      };
      applyRouteOptionsToChainSelects({
        preferredFrom: coin === "btc" ? "bitcoin" : "",
        preferredTo: coin === "btc" && equivalentAssets ? "ethereum" : "",
      });
      if (!state.routeResultsVisible || options.initial) {
        setRouteHint();
      }
    } catch (error) {
      state.routeOptions = null;
      fillSelect("#from-chain-select", state.chains, $("#from-chain-select").value || "bitcoin", displayChain);
      fillSelect("#to-chain-select", state.chains, $("#to-chain-select").value || "ethereum", displayChain);
      $("#find-route").disabled = false;
      if (!state.routeResultsVisible) {
        $("#route-results").innerHTML = `<p class="muted">Route options unavailable; using all chains for now.</p>`;
      }
    }
  }

  function applyRouteOptionsToChainSelects(options = {}) {
    if (!state.routeOptions) {
      return;
    }

    const fromSelect = $("#from-chain-select");
    const toSelect = $("#to-chain-select");
    const previousFrom = options.preserveFrom ? fromSelect.value : "";
    const previousTo = toSelect.value;
    const fromChains = state.routeOptions.fromChains;

    fillSelect("#from-chain-select", fromChains, previousFrom || options.preferredFrom || fromChains[0]?.slug || "", displayChain);
    if (!fromChains.some((chain) => chain.slug === fromSelect.value)) {
      fromSelect.value = options.preferredFrom && fromChains.some((chain) => chain.slug === options.preferredFrom)
        ? options.preferredFrom
        : fromChains[0]?.slug || "";
    }

    const toChains = routeToChainsForFrom(fromSelect.value);
    fillSelect("#to-chain-select", toChains, previousTo || options.preferredTo || toChains[0]?.slug || "", displayChain);
    if (!toChains.some((chain) => chain.slug === toSelect.value)) {
      toSelect.value = options.preferredTo && toChains.some((chain) => chain.slug === options.preferredTo)
        ? options.preferredTo
        : toChains[0]?.slug || "";
    }

    const hasOptions = fromChains.length > 0 && toChains.length > 0;
    fromSelect.disabled = !hasOptions;
    toSelect.disabled = !hasOptions;
    $("#find-route").disabled = !hasOptions;
    if (!hasOptions) {
      $("#route-results").innerHTML = `<p class="muted">No open route options for ${escapeHTML(displayCoin($("#coin-select").value))} yet.</p>`;
    }
  }

  function routeToChainsForFrom(fromChain) {
    if (!state.routeOptions) {
      return state.chains;
    }
    const toSlugs = new Set(
      state.routeOptions.pairs
        .filter((pair) => pair.fromChain === fromChain)
        .map((pair) => pair.toChain)
    );
    if (toSlugs.size === 0) {
      return [];
    }
    return state.routeOptions.toChains.filter((chain) => toSlugs.has(chain.slug));
  }

  function setRouteHint() {
    const coin = $("#coin-select").value;
    const fromCount = state.routeOptions?.fromChains?.length || 0;
    const pairCount = state.routeOptions?.pairs?.length || 0;
    if (fromCount === 0 || pairCount === 0) {
      $("#route-results").innerHTML = `<p class="muted">No open route options for ${escapeHTML(displayCoin(coin))} yet.</p>`;
      return;
    }
    $("#route-results").innerHTML = `<p class="muted">Pick from ${fromCount} valid source chain${fromCount === 1 ? "" : "s"} for ${escapeHTML(displayCoin(coin))}; destination chains update from live route options.</p>`;
  }

  function scheduleLiveRefresh() {
    clearTimeout(state.liveRefreshTimer);
    state.liveRefreshTimer = window.setTimeout(async () => {
      await refreshAll();
      if (!state.routeResultsVisible) {
        return;
      }
      try {
        renderRoutes(await loadRoutes());
      } catch (error) {
        $("#route-results").innerHTML = `<p class="error-text">${escapeHTML(error.message)}</p>`;
      }
    }, 350);
  }

  function renderEvents(error) {
    const list = $("#events-list");
    const count = $("#events-count");
    const visibleEvents = filteredEvents();
    count.textContent = `${visibleEvents.length} event${visibleEvents.length === 1 ? "" : "s"}`;

    if (error && visibleEvents.length === 0) {
      list.innerHTML = `<li class="event-empty">Events endpoint unavailable: ${escapeHTML(error.message)}</li>`;
      return;
    }

    if (visibleEvents.length === 0) {
      list.innerHTML = `<li class="event-empty">No matching rail events in the last 24h</li>`;
      return;
    }

    list.innerHTML = visibleEvents.slice(0, 50).map((event) => {
      const type = field(event, ["event_type", "eventType", "type"], "rail_event");
      const tone = eventTone(type);
      const exchange = displayExchange(field(event, ["exchange_slug", "exchange", "exchange.slug"], ""));
      const coin = displayCoin(field(event, ["coin_slug", "coin", "coin.slug", "coin.symbol"], ""));
      const chain = displayChain(field(event, ["chain_slug", "chain", "chain.slug", "chain.symbol"], ""));
      const occurred = field(event, ["occurred_at", "occurredAt", "time", "timestamp"], "");
      const summary = field(event, ["summary"], humanize(type));
      const changes = pickArray(event, ["changes"]).slice(0, 3);
      const showChanges = changes.length > 1;

      return `
        <li class="event-item">
          <span class="event-type ${tone}">${escapeHTML(humanize(type))}</span>
          <strong>${escapeHTML(exchange)} ${escapeHTML(coin)} / ${escapeHTML(chain)}</strong>
          <span class="event-summary">${escapeHTML(summary)}</span>
          ${showChanges ? `<div class="event-changes">${changes.map(renderEventChange).join("")}</div>` : ""}
          <small>${escapeHTML(relativeTime(occurred))}</small>
        </li>
      `;
    }).join("");
  }

  function renderEventChange(change) {
    const label = field(change, ["label"], "Change");
    const before = field(change, ["before"], "n/a");
    const after = field(change, ["after"], "n/a");
    const delta = field(change, ["delta"], "");
    const deltaPercent = field(change, ["delta_percent", "deltaPercent"], "");
    const direction = field(change, ["direction"], "change");
    const deltaText = [delta, deltaPercent].filter(Boolean).join(", ");
    return `<span class="event-change ${escapeHTML(direction)}">${escapeHTML(label)} ${escapeHTML(before)} -> ${escapeHTML(after)}${deltaText ? ` (${escapeHTML(deltaText)})` : ""}</span>`;
  }

  function populateRouteForm() {
    fillSelect("#coin-select", state.coins, "btc", displayCoin);
    fillSelect("#from-chain-select", state.chains, "bitcoin", displayChain);
    fillSelect("#to-chain-select", state.chains, "ethereum", displayChain);
    $("#amount-input").value = "0.1";
  }

  function populateEventFilters() {
    fillSelectWithAll("#event-exchange-filter", state.exchanges, "All exchanges", displayExchange);
    fillSelectWithAll("#event-coin-filter", state.coins, "All coins", displayCoin);
    fillSelectWithAll("#event-chain-filter", state.chains, "All chains", displayChain);
    $("#event-kind-filter").value = "availability";
  }

  function fillSelect(selector, items, preferred, formatter) {
    const select = $(selector);
    select.innerHTML = items.map((item) => {
      return `<option value="${escapeHTML(item.slug)}">${escapeHTML(formatter(item.slug))}</option>`;
    }).join("");
    if (items.some((item) => item.slug === preferred)) {
      select.value = preferred;
    }
  }

  function fillSelectWithAll(selector, items, allLabel, formatter) {
    const select = $(selector);
    const previous = select.value;
    select.innerHTML = `<option value="">${escapeHTML(allLabel)}</option>` + items.map((item) => {
      return `<option value="${escapeHTML(item.slug)}">${escapeHTML(formatter(item.slug))}</option>`;
    }).join("");
    if (items.some((item) => item.slug === previous)) {
      select.value = previous;
    }
  }

  function normalizeRouteOptionChains(payload, keys) {
    return pickArray(payload, keys).map((row) => {
      const slug = normalizeSlug(field(row, ["slug", "chain_slug", "chainSlug", "chain.slug", "chain"], ""));
      return {
        slug,
        symbol: field(row, ["symbol", "chain.symbol", "slug"], slug).toUpperCase(),
        name: field(row, ["name", "chain.name", "symbol", "slug"], slug),
      };
    }).filter((item) => item.slug);
  }

  function normalizeRouteOptionPairs(payload) {
    return pickArray(payload, ["pairs"]).map((row) => {
      return {
        fromChain: normalizeSlug(field(row, ["from_chain", "fromChain", "from_chain.slug", "fromChain.slug"], "")),
        toChain: normalizeSlug(field(row, ["to_chain", "toChain", "to_chain.slug", "toChain.slug"], "")),
      };
    }).filter((pair) => pair.fromChain && pair.toChain);
  }

  function deriveHotPairs() {
    const counts = new Map();
    for (const row of state.rails) {
      const coin = coinSlug(row);
      const chain = chainSlug(row);
      if (!coin || !chain) {
        continue;
      }
      const key = `${coin}|${chain}`;
      counts.set(key, (counts.get(key) || 0) + 1);
    }

    const priority = PRIORITY_PAIRS.map(([coin, chain], index) => ({
      coin,
      chain,
      score: (counts.get(`${coin}|${chain}`) || 0) + (100 - index),
    }));

    const discovered = Array.from(counts.entries()).map(([key, count]) => {
      const [coin, chain] = key.split("|");
      return { coin, chain, score: count };
    });

    const byKey = new Map();
    [...priority, ...discovered].forEach((pair) => {
      const key = `${pair.coin}|${pair.chain}`;
      const existing = byKey.get(key);
      if (!existing || existing.score < pair.score) {
        byKey.set(key, pair);
      }
    });

    return Array.from(byKey.values()).sort((a, b) => b.score - a.score);
  }

  function addEvent(event) {
    if (!eventMatchesFilters(event)) {
      return;
    }
    const id = field(event, ["id"], "");
    const occurred = field(event, ["occurred_at", "occurredAt"], "");
    const key = id ? String(id) : `${occurred}-${field(event, ["event_type", "eventType", "type"], "")}`;
    if (state.eventIds.has(key)) {
      return;
    }
    state.eventIds.add(key);
    state.events.unshift(event);
    state.events = state.events.slice(0, 50);
  }

  function readEventFilters() {
    state.eventFilters = {
      exchange: normalizeSlug($("#event-exchange-filter").value),
      coin: normalizeSlug($("#event-coin-filter").value),
      chain: normalizeSlug($("#event-chain-filter").value),
      kind: $("#event-kind-filter").value || "availability",
    };
  }

  function resetEventsFeed() {
    state.events = [];
    state.eventIds = new Set();
    state.lastCursor = null;
    renderEvents();
  }

  function eventQueryParams() {
    const params = new URLSearchParams({ limit: "500", since: EVENTS_WINDOW });
    if (state.eventFilters.exchange) {
      params.set("exchange", state.eventFilters.exchange);
    }
    if (state.eventFilters.coin) {
      params.set("coin", state.eventFilters.coin);
    }
    if (state.eventFilters.chain) {
      params.set("chain", state.eventFilters.chain);
    }
    const selectedTypes = eventTypeFilterValues();
    for (const type of selectedTypes) {
      params.append("event_type", type);
    }
    return params;
  }

  function eventSocketFilters() {
    const filters = {};
    if (state.eventFilters.exchange) {
      filters.exchange = [state.eventFilters.exchange];
    }
    if (state.eventFilters.coin) {
      filters.coin = [state.eventFilters.coin];
    }
    if (state.eventFilters.chain) {
      filters.chain = [state.eventFilters.chain];
    }
    const selectedTypes = eventTypeFilterValues();
    if (selectedTypes.length > 0) {
      filters.event_type = selectedTypes;
    }
    return filters;
  }

  function eventTypeFilterValues() {
    if (state.eventFilters.kind === "all") {
      return [];
    }
    if (state.eventFilters.kind === "availability") {
      return AVAILABILITY_EVENT_TYPES;
    }
    return [state.eventFilters.kind];
  }

  function filteredEvents() {
    return state.events.filter(eventMatchesFilters);
  }

  function eventMatchesFilters(event) {
    if (state.eventFilters.exchange && state.eventFilters.exchange !== exchangeSlug(event)) {
      return false;
    }
    if (state.eventFilters.coin && state.eventFilters.coin !== coinSlug(event)) {
      return false;
    }
    if (state.eventFilters.chain && state.eventFilters.chain !== chainSlug(event)) {
      return false;
    }
    const selectedTypes = eventTypeFilterValues();
    if (selectedTypes.length > 0) {
      const type = normalizeSlug(field(event, ["event_type", "eventType", "type"], ""));
      return selectedTypes.includes(type);
    }
    return true;
  }

  function unwrapEvent(payload) {
    if (payload.event) {
      return payload.event;
    }
    if (payload.data && !Array.isArray(payload.data)) {
      return payload.data;
    }
    if (field(payload, ["event_type", "eventType"], "")) {
      return payload;
    }
    return null;
  }

  async function getJSON(url) {
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
    });
    const text = await response.text();
    const payload = text ? parseJSON(text) : null;

    if (!response.ok) {
      throw new Error(errorMessage(payload, response));
    }
    return payload;
  }

  function errorMessage(payload, response) {
    if (payload) {
      const message = field(payload, ["error", "message", "detail"], "");
      if (message) {
        return message;
      }
    }
    return `${response.status} ${response.statusText}`;
  }

  function normalizeReference(payload, keys, kind) {
    return pickArray(payload, keys).map((row) => {
      const slug = normalizeSlug(field(row, ["slug", `${kind}_slug`, "id", "symbol", "name"], ""));
      return {
        slug,
        symbol: field(row, ["symbol", "code", "slug", "name"], slug).toUpperCase(),
        name: field(row, ["name", "title", "symbol", "slug"], slug),
      };
    }).filter((item) => item.slug);
  }

  function mergeReference(fallback, incoming) {
    const bySlug = new Map();
    [...fallback, ...incoming].forEach((item) => {
      if (item.slug) {
        bySlug.set(item.slug, item);
      }
    });
    return Array.from(bySlug.values());
  }

  function pickArray(payload, keys) {
    if (Array.isArray(payload)) {
      return payload;
    }
    if (!payload || typeof payload !== "object") {
      return [];
    }
    for (const key of keys) {
      const value = rawField(payload, [key], null);
      if (Array.isArray(value)) {
        return value;
      }
    }
    if (payload.data && typeof payload.data === "object") {
      if (Array.isArray(payload.data)) {
        return payload.data;
      }
      for (const key of keys) {
        const value = rawField(payload.data, [key], null);
        if (Array.isArray(value)) {
          return value;
        }
      }
    }
    return [];
  }

  function exchangeSlug(row) {
    return normalizeSlug(field(row, ["exchange_slug", "exchangeSlug", "exchange.slug", "exchange"], ""));
  }

  function coinSlug(row) {
    return normalizeSlug(field(row, ["coin_slug", "coinSlug", "coin.slug", "coin.symbol", "coin"], ""));
  }

  function chainSlug(row) {
    return normalizeSlug(field(row, ["chain_slug", "chainSlug", "chain.slug", "chain.symbol", "chain"], ""));
  }

  function displayExchange(slug) {
    const normalized = normalizeSlug(slug);
    const exchange = state.exchanges.find((item) => item.slug === normalized);
    return exchange ? exchange.name : titleize(normalized || slug || "Exchange");
  }

  function displayCoin(slug) {
    const normalized = normalizeSlug(slug);
    const coin = state.coins.find((item) => item.slug === normalized);
    return coin ? coin.symbol : String(slug || "COIN").toUpperCase();
  }

  function displayChain(slug) {
    const normalized = normalizeSlug(slug);
    const chain = state.chains.find((item) => item.slug === normalized);
    return chain ? chain.symbol : titleize(normalized || slug || "Chain");
  }

  function pairLabel(pair) {
    return `${displayCoin(pair.coin)} / ${displayChain(pair.chain)}`;
  }

  function eventTone(type) {
    if (String(type).includes("_on") || String(type).includes("relisted")) {
      return "up";
    }
    if (String(type).includes("_off") || String(type).includes("delisted")) {
      return "down";
    }
    return "change";
  }

  function humanize(value) {
    return String(value || "")
      .replaceAll("_", " ")
      .replace(/\b\w/g, (letter) => letter.toUpperCase());
  }

  function titleize(value) {
    return String(value || "")
      .replaceAll("-", " ")
      .replaceAll("_", " ")
      .replace(/\b\w/g, (letter) => letter.toUpperCase());
  }

  function relativeTime(value) {
    if (!value) {
      return "never";
    }
    const time = new Date(value).getTime();
    if (!Number.isFinite(time)) {
      return String(value);
    }
    const seconds = Math.max(0, Math.round((Date.now() - time) / 1000));
    if (seconds < 10) {
      return "just now";
    }
    if (seconds < 60) {
      return `${seconds}s ago`;
    }
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) {
      return `${minutes}m ago`;
    }
    const hours = Math.round(minutes / 60);
    if (hours < 24) {
      return `${hours}h ago`;
    }
    const days = Math.round(hours / 24);
    return `${days}d ago`;
  }

  function boolField(obj, keys, fallback) {
    const value = rawField(obj, keys, fallback);
    if (typeof value === "boolean") {
      return value;
    }
    if (typeof value === "number") {
      return value !== 0;
    }
    if (typeof value === "string") {
      return ["true", "1", "yes", "enabled", "open", "active"].includes(value.toLowerCase());
    }
    return Boolean(value);
  }

  function field(obj, keys, fallback = "") {
    const value = rawField(obj, keys, fallback);
    if (value === null || value === undefined) {
      return fallback;
    }
    if (typeof value === "object") {
      return value.slug || value.symbol || value.name || fallback;
    }
    return value;
  }

  function rawField(obj, keys, fallback = "") {
    for (const key of keys) {
      const value = readPath(obj, key);
      if (value !== undefined && value !== null && value !== "") {
        return value;
      }
    }
    return fallback;
  }

  function readPath(obj, path) {
    if (!obj || typeof obj !== "object") {
      return undefined;
    }
    return String(path).split(".").reduce((current, part) => {
      if (current && Object.prototype.hasOwnProperty.call(current, part)) {
        return current[part];
      }
      return undefined;
    }, obj);
  }

  function normalizeSlug(value) {
    return String(value || "")
      .trim()
      .toLowerCase()
      .replace(/\s+/g, "-");
  }

  function setPill(selector, text, tone) {
    const element = $(selector);
    element.textContent = text;
    element.className = `pill ${tone}`;
  }

  function parseJSON(text) {
    try {
      return JSON.parse(text);
    } catch (_) {
      return null;
    }
  }

  function escapeHTML(value) {
    return String(value)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#039;");
  }
})();
