/* global React, ReactDOM, TweaksPanel, useTweaks, TweakSection, TweakRadio, TweakSelect, TweakToggle */

const { useState, useEffect, useMemo, useRef, useCallback } = React;
const D = window.RAILDEX_DATA;

// ---------- utilities ----------
const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "shell": "red",
  "screen": "green",
  "scanlines": true,
  "ledPulse": true
}/*EDITMODE-END*/;

function pad(n, w = 2) { return String(n).padStart(w, "0"); }
function nowClock() {
  const d = new Date();
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}
function rel(ts) {
  const s = Math.max(0, Math.round((Date.now() - ts) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  return `${Math.round(s / 3600)}h ago`;
}
function freshnessLabel(ts) {
  const s = (Date.now() - ts) / 1000;
  if (s < 30) return { label: "FRESH", cls: "ok" };
  if (s < 120) return { label: "LIVE", cls: "ok" };
  if (s < 360) return { label: "STALE", cls: "warn" };
  return { label: "OFFLINE", cls: "bad" };
}
function tierToCls(t) { return t === "S" ? "ok" : t === "A" ? "ok" : t === "B" ? "warn" : "bad"; }
function statusToCls(s) { return s === "OPEN" ? "ok" : s === "DEGRADED" ? "warn" : "bad"; }

// ---------- DEVICE CHROME ----------
function HeaderStrip({ pulse, stats, view, setView, onTweaks }) {
  const items = [
    { id: "rails", num: "I",   label: "RAILS",  sub: "STATUS" },
    { id: "route", num: "II",  label: "ROUTE",  sub: "FIND" },
    { id: "feed",  num: "III", label: "FEED",   sub: "LIVE" },
  ];
  return (
    <div className="header-strip">
      <div className={"led-main" + (pulse ? " pulse" : "")} />
      <div className="nameplate">
        <span>RAIL-DEX</span>
      </div>
      <div className="tabs">
        {items.map(it => (
          <div
            key={it.id}
            className={"tab" + (view === it.id ? " active" : "")}
            onClick={() => setView(it.id)}
          >
            {it.label}<small>{it.num}</small>
          </div>
        ))}
      </div>
      <div className="status-pill">
        <span className="seg"><span className="dot" /><b>ONLINE</b></span>
        <span className="sep">·</span>
        <span className="seg"><b>RAILS</b>{stats.healthy}/{stats.rails}</span>
        <span className="sep">·</span>
        <span className="seg">{nowClock()}</span>
      </div>
      <button className="gear-btn" title="Tweaks" onClick={onTweaks}>⚙</button>
    </div>
  );
}

// ---------- VIEW 1: RAILS (status board) ----------
function RailsView({ railFilter, setRailFilter, selectedRailId, setSelectedRailId, listRef, rowRefs }) {
  const allRails = D.allRails;
  const exchanges = D.exchanges;
  const coins = D.coins;
  const chains = D.chains;

  // Apply filters
  const filtered = allRails.filter(r =>
    (railFilter.exchange === "ALL" || r.exchange === railFilter.exchange) &&
    (railFilter.coin === "ALL" || r.coin === railFilter.coin) &&
    (railFilter.chain === "ALL" || r.chain === railFilter.chain) &&
    (railFilter.status === "ALL" || r.status === railFilter.status)
  );

  const selected = filtered.find(r => r.id === selectedRailId) || filtered[0];

  // Counts
  const openCount = filtered.filter(r => r.status === "OPEN").length;
  const warnCount = filtered.filter(r => r.status === "DEGRADED").length;
  const offCount  = filtered.filter(r => r.status === "OFFLINE").length;

  const Chip = ({ label, active, onClick }) => (
    <span className={"filter-chip" + (active ? " active" : "")} onClick={onClick}>{label}</span>
  );

  return (
    <>
      <div className="lcd-header">
        <div className="crumb">
          <span>STATUS BOARD</span>
          <span className="sep">/</span>
          <span style={{ color: "var(--lcd-fg)" }}>{filtered.length} RAILS</span>
          <span className="sep">·</span>
          <span style={{ color: "var(--lcd-ok)" }}>{openCount} OPEN</span>
          <span className="sep">·</span>
          <span style={{ color: "var(--lcd-warn)" }}>{warnCount} DEGR</span>
          <span className="sep">·</span>
          <span style={{ color: "var(--lcd-bad)" }}>{offCount} OFF</span>
        </div>
      </div>

      <div className="lcd-body lcd-rails-v2">
        {/* FEATURED: selected rail detail */}
        {selected && (
          <div className="rail-featured">
            <div className="rail-featured-main">
              <div className="rf-exchange">{selected.exchange}</div>
              <div className="rf-pair">
                <span className="rf-coin">{selected.coin}</span>
                <span className="rf-on">on</span>
                <span className="rf-chain">{selected.chainName}</span>
              </div>
              <div className="rf-tags">
                <span className={"badge " + statusToCls(selected.status)}>{selected.status}</span>
                <span className="badge tier">TIER {selected.tier}</span>
                <span className="badge region">{selected.region}</span>
              </div>
            </div>
            <div className="rail-featured-grid">
              <div className="rf-stat"><span className="lbl">DEPOSIT</span><span className={"val " + statusToCls(selected.deposit)}>{selected.deposit}</span></div>
              <div className="rf-stat"><span className="lbl">WITHDRAW</span><span className={"val " + statusToCls(selected.withdraw)}>{selected.withdraw}</span></div>
              <div className="rf-stat"><span className="lbl">FEE</span><span className="val">{selected.feeFlat} {selected.coin}</span></div>
              <div className="rf-stat"><span className="lbl">ETA</span><span className="val">{Math.round(selected.eta/60)}m</span></div>
              <div className="rf-stat"><span className="lbl">MIN WD</span><span className="val">{selected.minWd} {selected.coin}</span></div>
              <div className="rf-stat"><span className="lbl">MAX WD</span><span className="val">{selected.maxWd.toLocaleString()} {selected.coin}</span></div>
              <div className="rf-stat"><span className="lbl">FINALITY</span><span className="val">~{selected.finality}s</span></div>
              <div className="rf-stat"><span className="lbl">FRESHNESS</span><span className="val">{rel(selected.freshness)}</span></div>
            </div>
          </div>
        )}

        {/* FILTERS */}
        <div className="rail-filters">
          <div className="filter-group">
            <span className="filter-label">EX</span>
            <Chip label="ALL" active={railFilter.exchange === "ALL"} onClick={() => setRailFilter(f => ({ ...f, exchange: "ALL" }))} />
            {exchanges.map(e => (
              <Chip key={e.code} label={e.code} active={railFilter.exchange === e.code} onClick={() => setRailFilter(f => ({ ...f, exchange: e.code }))} />
            ))}
          </div>
          <div className="filter-group">
            <span className="filter-label">COIN</span>
            <Chip label="ALL" active={railFilter.coin === "ALL"} onClick={() => setRailFilter(f => ({ ...f, coin: "ALL" }))} />
            {coins.map(c => (
              <Chip key={c.sym} label={c.sym} active={railFilter.coin === c.sym} onClick={() => setRailFilter(f => ({ ...f, coin: c.sym }))} />
            ))}
          </div>
          <div className="filter-group">
            <span className="filter-label">CHAIN</span>
            <Chip label="ALL" active={railFilter.chain === "ALL"} onClick={() => setRailFilter(f => ({ ...f, chain: "ALL" }))} />
            {chains.map(ch => (
              <Chip key={ch.id} label={ch.id} active={railFilter.chain === ch.id} onClick={() => setRailFilter(f => ({ ...f, chain: ch.id }))} />
            ))}
          </div>
        </div>

        {/* TABLE */}
        <div className="rail-table" ref={listRef}>
          <div className="rail-row rail-head">
            <span>EX</span>
            <span>COIN</span>
            <span>CHAIN</span>
            <span>DEP</span>
            <span>WD</span>
            <span>FEE</span>
            <span>MIN</span>
            <span>MAX</span>
            <span>ETA</span>
            <span>FRESH</span>
          </div>
          {filtered.length === 0 && (
            <div className="rail-empty">&gt; NO RAILS MATCH FILTERS</div>
          )}
          {filtered.map((r, i) => (
            <div
              key={r.id}
              className={"rail-row" + (selected && r.id === selected.id ? " active" : "")}
              ref={el => (rowRefs.current[i] = el)}
              onClick={() => setSelectedRailId(r.id)}
            >
              <span className="rl-ex">{r.exchange}</span>
              <span className="rl-coin">{r.coin}</span>
              <span className="rl-chain">{r.chain}</span>
              <span className={"rl-st " + statusToCls(r.deposit)}>{r.deposit === "OPEN" ? "●" : r.deposit === "DEGRADED" ? "◐" : "○"}</span>
              <span className={"rl-st " + statusToCls(r.withdraw)}>{r.withdraw === "OPEN" ? "●" : r.withdraw === "DEGRADED" ? "◐" : "○"}</span>
              <span className="rl-fee">{r.feeFlat} {r.coin}</span>
              <span className="rl-min">{r.minWd}</span>
              <span className="rl-max">{r.maxWd >= 1000 ? Math.round(r.maxWd/1000) + "k" : r.maxWd}</span>
              <span className="rl-eta">{Math.round(r.eta/60)}m</span>
              <span className="rl-fresh">{rel(r.freshness)}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="lcd-footer">
        <div className="keys">
          <span className="key">↑↓ SCROLL</span>
          <span className="key">←→ VIEW</span>
          <span className="key">CLICK CHIP TO FILTER · CLICK ROW TO INSPECT</span>
        </div>
        <span>RAIL-DEX OS · v1.2</span>
      </div>
    </>
  );
}

// ---------- VIEW 2: ROUTE FINDER ----------
function RouteView({ form, setForm, onSearch, results, focus, setFocus }) {
  const coins = D.coins.map(c => c.sym);
  const chains = D.chains.map(c => c.id);
  return (
    <>
      <div className="lcd-header">
        <div className="crumb">
          <span>ROUTE FINDER</span>
          <span className="sep">/</span>
          <span style={{ color: "var(--lcd-fg)" }}>{form.coin}</span>
          <span className="sep">·</span>
          <span style={{ color: "var(--lcd-fg)" }}>{form.from} → {form.to}</span>
          <span className="sep">/</span>
          <span style={{ color: "var(--lcd-fg-bright)" }}>{results.length} MATCH{results.length === 1 ? "" : "ES"}</span>
        </div>
      </div>
      <div className="lcd-body">
        <div className="form">
          <div className="field-input">
            <label>COIN</label>
            <div className="chip-row">
              {coins.map(s => (
                <span key={s} className={"chip" + (form.coin === s ? " active" : "")} onClick={() => setForm(f => ({ ...f, coin: s }))}>{s}</span>
              ))}
            </div>
          </div>
          <div className="field-input">
            <label>AMOUNT</label>
            <input
              className="amount-input"
              value={form.amount}
              onChange={e => setForm(f => ({ ...f, amount: e.target.value }))}
              placeholder="1000"
            />
          </div>
          <div className="field-input">
            <label>FROM CHAIN</label>
            <div className="chip-row">
              {chains.map(s => (
                <span key={s} className={"chip" + (form.from === s ? " active" : "")} onClick={() => setForm(f => ({ ...f, from: s }))}>{s}</span>
              ))}
            </div>
          </div>
          <div className="field-input">
            <label>TO CHAIN</label>
            <div className="chip-row">
              {chains.map(s => (
                <span key={s} className={"chip" + (form.to === s ? " active" : "")} onClick={() => setForm(f => ({ ...f, to: s }))}>{s}</span>
              ))}
            </div>
          </div>
        </div>

        <div className="route-results">
          <div className="rr-head">
            <span>RK</span><span>EXCH</span><span>NOTES</span><span>FEE · ETA</span><span>STATUS</span>
          </div>
          {results.length === 0 && (
            <div className="rr-empty">
              &gt; SELECT COIN + CHAINS · PRESS A TO RANK ROUTES
            </div>
          )}
          {results.map((r, i) => (
            <div key={r.exchange + i} className={"rr-row" + (i === 0 ? " best" : "")}>
              <span className="rk">{i === 0 ? "★" : `#${i + 1}`}</span>
              <span className="rr-exchange">{r.exchange}</span>
              <span>{r.both ? "DEP + WD" : r.kind === "withdraw" ? "WD ONLY" : "DEP ONLY"}</span>
              <span>{r.feeFlat} {r.feeCoin} · {Math.round(r.eta / 60)}m</span>
              <span className={"badge " + statusToCls(r.status)}>{r.status === "OPEN" ? "OPEN" : r.status === "DEGRADED" ? "DEGR" : "OFF"}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="lcd-footer">
        <div className="keys">
          <span className="key">←→ SWITCH VIEW</span>
          <span className="key">RANKED BY FLAT WITHDRAWAL FEE + ETA</span>
        </div>
        <span>RESULTS UPDATE LIVE</span>
      </div>
    </>
  );
}

// ---------- VIEW 3: LIVE FEED ----------
function FeedView({ events }) {
  return (
    <>
      <div className="lcd-header">
        <div className="crumb">
          <span>LIVE FEED</span>
          <span className="sep">/</span>
          <span style={{ color: "var(--lcd-fg)" }}>{events.length} EVENTS</span>
          <span className="sep">·</span>
          <span className="live-dot" /><span style={{ color: "var(--lcd-fg-bright)" }}>STREAMING</span>
        </div>
      </div>
      <div className="lcd-body">
        {events.map((e, i) => (
          <div key={e.id} className={"feed-line" + (e.fresh ? " new" : "")}>
            <span className="ftime">{rel(e.t)}</span>
            <span className={"fsev " + e.sev}>{e.sev.toUpperCase()}</span>
            <span className="fex">{e.ex}</span>
            <span className="fmsg">&gt; {e.msg}</span>
          </div>
        ))}
      </div>
      <div className="lcd-footer">
        <div className="keys">
          <span className="key">WS · LIVE</span>
          <span className="key">←→ SWITCH VIEW</span>
        </div>
        <span>STREAM /events</span>
      </div>
    </>
  );
}

// ---------- MAIN APP ----------
function App() {
  const [tweaks, setTweak] = useTweaks(TWEAK_DEFAULTS);
  const [view, setView] = useState("rails");

  // Persist theme onto <body>
  useEffect(() => { document.body.setAttribute("data-shell", tweaks.shell); }, [tweaks.shell]);
  useEffect(() => { document.body.setAttribute("data-screen", tweaks.screen); }, [tweaks.screen]);

  // Boot splash
  const [booted, setBooted] = useState(false);
  useEffect(() => { const t = setTimeout(() => setBooted(true), 600); return () => clearTimeout(t); }, []);

  // Rails view selection
  const [railFilter, setRailFilter] = useState({ exchange: "ALL", coin: "ALL", chain: "ALL", status: "ALL" });
  const [selectedRailId, setSelectedRailId] = useState(D.allRails[0]?.id || null);
  const listRef = useRef(null);
  const rowRefs = useRef([]);

  // Route view state
  const [form, setForm] = useState({ coin: "USDC", from: "ETH", to: "ARB", amount: "1000" });
  const [results, setResults] = useState([]);
  const [focus, setFocus] = useState(0);

  const computeRoutes = useCallback(() => {
    const fromList = D.supportRails[`${form.coin}/${form.from}`] || [];
    const toList   = D.supportRails[`${form.coin}/${form.to}`]   || [];
    const fromMap  = new Map(fromList.map(r => [r.exchange, r]));
    const merged = [];
    for (const r of toList) {
      const dep = fromMap.get(r.exchange);
      if (dep) {
        merged.push({
          exchange: r.exchange,
          feeFlat: r.feeFlat,
          feeCoin: r.feeCoin,
          eta: Math.max(r.eta, dep.eta),
          status: r.status === "OFFLINE" || dep.status === "OFFLINE" ? "OFFLINE"
                : r.status === "DEGRADED" || dep.status === "DEGRADED" ? "DEGRADED" : "OPEN",
          both: true,
          kind: "both",
        });
      }
    }
    // Sort: OPEN first, then by score (fee + eta penalty)
    merged.sort((a, b) => {
      const sa = (a.status === "OPEN" ? 0 : a.status === "DEGRADED" ? 1 : 2);
      const sb = (b.status === "OPEN" ? 0 : b.status === "DEGRADED" ? 1 : 2);
      if (sa !== sb) return sa - sb;
      return (a.feeFlat - b.feeFlat) || (a.eta - b.eta);
    });
    setResults(merged.slice(0, 8));
  }, [form]);

  useEffect(() => { computeRoutes(); }, [computeRoutes]);

  // Feed view state — seeded + auto-appended events
  const [events, setEvents] = useState(() => D.feedSeed.map((e, i) => ({ ...e, id: "seed-" + i, fresh: false })));
  useEffect(() => {
    if (view !== "feed") return;
    const interval = setInterval(() => {
      const pool = [
        { sev: "info", msg: "ORDERBOOK SNAPSHOT REFRESHED" },
        { sev: "ok",   msg: "WITHDRAWALS RESUMED · USDC ON ARB" },
        { sev: "warn", msg: "QUEUE DEPTH > 200 · ETH" },
        { sev: "info", msg: "FEE TIER UPDATE · MAKER 0.10%" },
        { sev: "ok",   msg: "DEPOSIT CREDITED · 12 CONF" },
        { sev: "warn", msg: "RAIL SLOW · 18m DELAY" },
        { sev: "info", msg: "NEW PAIR LISTED" },
      ];
      const ex = D.exchanges[Math.floor(Math.random() * D.exchanges.length)];
      const e = pool[Math.floor(Math.random() * pool.length)];
      setEvents(prev => [{ ...e, ex: ex.code, t: Date.now(), id: "evt-" + Date.now(), fresh: true }, ...prev].slice(0, 60));
    }, 4500);
    return () => clearInterval(interval);
  }, [view]);

  // periodic re-render so relative times tick
  const [, forceTick] = useState(0);
  useEffect(() => { const i = setInterval(() => forceTick(t => t + 1), 1000); return () => clearInterval(i); }, []);

  // Aggregate stats for mini-screen
  const stats = useMemo(() => {
    const rails = D.exchanges.length;
    const healthy = D.exchanges.filter(e => e.withdrawals === "OPEN").length;
    return { rails, healthy };
  }, []);

  // D-pad / keyboard controls
  const tabs = ["rails", "route", "feed"];
  const onLeft = () => setView(v => tabs[(tabs.indexOf(v) + 2) % 3]);
  const onRight = () => setView(v => tabs[(tabs.indexOf(v) + 1) % 3]);
  const onUp = () => {
    // no-op (scroll handled by overflow)
  };
  const onDown = () => {
    // no-op
  };
  const onA = () => {
    if (view === "route") computeRoutes();
    else if (view === "feed") {
      // toggle pause - simple: not implemented as state, just blink LED feel
    }
  };
  const onB = () => setView("rails");

  // Scroll selected rail into view
  useEffect(() => {
    if (view !== "rails") return;
    const idx = D.allRails.findIndex(r => r.id === selectedRailId);
    if (idx < 0) return;
    const el = rowRefs.current[idx];
    if (el && listRef.current) {
      const lc = listRef.current;
      const er = el.getBoundingClientRect();
      const lr = lc.getBoundingClientRect();
      if (er.top < lr.top) lc.scrollTop -= (lr.top - er.top) + 4;
      else if (er.bottom > lr.bottom) lc.scrollTop += (er.bottom - lr.bottom) + 4;
    }
  }, [selectedRailId, view]);

  // Keyboard
  useEffect(() => {
    const onKey = (e) => {
      if (e.target && (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA")) return;
      if (e.key === "ArrowLeft") { onLeft(); e.preventDefault(); }
      else if (e.key === "ArrowRight") { onRight(); e.preventDefault(); }
      else if (e.key === "ArrowUp") { onUp(); e.preventDefault(); }
      else if (e.key === "ArrowDown") { onDown(); e.preventDefault(); }
      else if (e.key === "a" || e.key === "A" || e.key === "Enter") onA();
      else if (e.key === "b" || e.key === "B" || e.key === "Escape") onB();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  // Fade fresh feed items
  useEffect(() => {
    if (!events.some(e => e.fresh)) return;
    const t = setTimeout(() => setEvents(prev => prev.map(e => ({ ...e, fresh: false }))), 1600);
    return () => clearTimeout(t);
  }, [events]);

  const openTweaks = () => window.postMessage({ type: '__activate_edit_mode' }, '*');

  return (
    <div className="stage">
      <div className="device">
        <HeaderStrip pulse={tweaks.ledPulse} stats={stats} view={view} setView={setView} onTweaks={openTweaks} />

        <div className="screen-panel">
          <div className={"lcd" + (tweaks.scanlines ? "" : " no-scanlines")}>
            {!booted && (
              <div className="boot">
                <div className="boot-inner">
                  <h1>RAIL-DEX</h1>
                  <div className="sub">LOADING ADAPTERS...</div>
                </div>
              </div>
            )}
            <div className="lcd-inner">
              {view === "rails" && (
                <RailsView railFilter={railFilter} setRailFilter={setRailFilter} selectedRailId={selectedRailId} setSelectedRailId={setSelectedRailId} listRef={listRef} rowRefs={rowRefs} />
              )}
              {view === "route" && (
                <RouteView form={form} setForm={setForm} onSearch={computeRoutes} results={results} focus={focus} setFocus={setFocus} />
              )}
              {view === "feed" && (
                <FeedView events={events} />
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Tweaks panel */}
      <TweaksPanel title="TWEAKS">
        <TweakSection title="Shell color">
          <TweakSelect value={tweaks.shell} onChange={v => setTweak("shell", v)} options={[
            { value: "red",    label: "Crimson (classic)" },
            { value: "blue",   label: "Cobalt" },
            { value: "yellow", label: "Mustard" },
            { value: "mint",   label: "Mint" },
            { value: "black",  label: "Slate" },
          ]} />
        </TweakSection>
        <TweakSection title="LCD tint">
          <TweakRadio value={tweaks.screen} onChange={v => setTweak("screen", v)} options={[
            { value: "green", label: "Green" },
            { value: "amber", label: "Amber" },
            { value: "blue",  label: "Blue" },
            { value: "mono",  label: "Mono" },
          ]} />
        </TweakSection>
        <TweakSection title="Effects">
          <TweakToggle value={tweaks.scanlines} onChange={v => setTweak("scanlines", v)} label="CRT scanlines" />
          <TweakToggle value={tweaks.ledPulse} onChange={v => setTweak("ledPulse", v)} label="Pulse main LED" />
        </TweakSection>
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
