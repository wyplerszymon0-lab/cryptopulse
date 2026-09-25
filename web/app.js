"use strict";

const SVG_NS = "http://www.w3.org/2000/svg";
const STRATEGIES = [
  { key: "optimized", name: "Walk-forward optimised", short: "Optimised", color: "--series-1" },
  { key: "fixed_default", name: "Fixed thresholds (±0.2)", short: "Fixed", color: "--series-2" },
  { key: "buy_hold", name: "Buy & hold", short: "Buy & hold", color: "--series-3" },
];

const state = { data: null, coin: null };
const $ = (id) => document.getElementById(id);
const cssVar = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

const fmtUSD = (v) =>
  "$" + v.toLocaleString("en-US", { maximumFractionDigits: v >= 100 ? 0 : v >= 1 ? 2 : 4, minimumFractionDigits: v >= 100 ? 0 : 2 });
const fmtPct = (v) => (v > 0 ? "+" : v < 0 ? "−" : "") + Math.abs(v).toFixed(1) + "%";
const fmtNum = (v) => (v < 0 ? "−" : "") + Math.abs(v).toFixed(2);
const fmtDate = (ms) => new Date(ms).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric" });
const coinName = (id) => id.charAt(0).toUpperCase() + id.slice(1);

function el(tag, attrs = {}, text) {
  const node = tag.startsWith("svg:") ? document.createElementNS(SVG_NS, tag.slice(4)) : document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, v);
  if (text !== undefined) node.textContent = text;
  return node;
}

// ── theme ────────────────────────────────────────────────────────────────────

function initTheme() {
  try {
    const saved = localStorage.getItem("theme");
    if (saved) document.documentElement.dataset.theme = saved;
  } catch (_) { /* storage unavailable */ }
  $("theme").addEventListener("click", () => {
    const dark = document.documentElement.dataset.theme
      ? document.documentElement.dataset.theme === "dark"
      : matchMedia("(prefers-color-scheme: dark)").matches;
    const next = dark ? "light" : "dark";
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem("theme", next); } catch (_) { /* ignore */ }
    render();
  });
}

// ── line chart ───────────────────────────────────────────────────────────────

function niceTicks(min, max, count) {
  const span = max - min || Math.abs(max) || 1;
  const step0 = span / count;
  const mag = 10 ** Math.floor(Math.log10(step0));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((s) => s >= step0);
  const ticks = [];
  for (let v = Math.floor(min / step) * step; v <= max + step * 1e-9; v += step) ticks.push(v);
  if (ticks[ticks.length - 1] < max) ticks.push(ticks[ticks.length - 1] + step);
  return ticks;
}

function monthTicks(times) {
  const out = [];
  let last = -1;
  times.forEach((t, i) => {
    const d = new Date(t);
    if (d.getUTCMonth() !== last && d.getUTCDate() <= 7) out.push(i);
    last = d.getUTCMonth();
  });
  return out;
}

/**
 * Draw a line chart with a crosshair tooltip.
 * series: [{ name, color (css var), values }], all aligned to `times`.
 */
function lineChart(container, { times, series, yFormat, shadeFrom, endLabels }) {
  container.replaceChildren();
  const width = Math.max(container.clientWidth, 280);
  const narrow = width < 560;
  const height = narrow ? 240 : 300;
  const pad = { top: 12, right: endLabels && !narrow ? 140 : 12, bottom: 26, left: narrow ? 52 : 64 };
  const innerW = width - pad.left - pad.right;
  const innerH = height - pad.top - pad.bottom;

  const all = series.flatMap((s) => s.values);
  const yTicks = niceTicks(Math.min(...all), Math.max(...all), narrow ? 4 : 5);
  const y0 = yTicks[0], y1 = yTicks[yTicks.length - 1];
  const n = times.length;
  const x = (i) => pad.left + (n === 1 ? 0 : (i / (n - 1)) * innerW);
  const y = (v) => pad.top + innerH - ((v - y0) / (y1 - y0)) * innerH;

  const svg = el("svg:svg", {
    viewBox: `0 0 ${width} ${height}`, width, height, tabindex: "0", role: "img",
    "aria-label": `${series.map((s) => s.name).join(", ")} from ${fmtDate(times[0])} to ${fmtDate(times[n - 1])}. Use arrow keys to inspect values.`,
  });

  if (shadeFrom !== undefined) {
    svg.append(el("svg:rect", { class: "oos", x: x(shadeFrom), y: pad.top, width: x(n - 1) - x(shadeFrom), height: innerH }));
    svg.append(el("svg:text", { class: "oos-label", x: x(shadeFrom) + 6, y: pad.top + 14 }, "out-of-sample"));
  }

  for (const v of yTicks) {
    svg.append(el("svg:line", { class: v === y0 ? "baseline" : "grid", x1: pad.left, x2: pad.left + innerW, y1: y(v), y2: y(v) }));
    svg.append(el("svg:text", { class: "tick", x: pad.left - 8, y: y(v) + 4, "text-anchor": "end" }, yFormat(v)));
  }

  const months = monthTicks(times);
  const every = Math.ceil(months.length / (narrow ? 4 : 8));
  months.forEach((i, k) => {
    if (k % every) return;
    const label = new Date(times[i]).toLocaleDateString("en-GB", { month: "short", year: "2-digit" });
    svg.append(el("svg:text", { class: "tick", x: x(i), y: height - 6, "text-anchor": "middle" }, label));
  });

  for (const s of series) {
    const d = s.values.map((v, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${y(v).toFixed(1)}`).join("");
    svg.append(el("svg:path", { class: "line", d, stroke: cssVar(s.color) }));
  }

  if (endLabels && !narrow) {
    // Direct labels at the line ends, nudged apart so they never overlap.
    const labels = series
      .map((s) => ({ s, y: y(s.values[n - 1]) }))
      .sort((a, b) => a.y - b.y);
    for (let i = 1; i < labels.length; i++) labels[i].y = Math.max(labels[i].y, labels[i - 1].y + 16);
    for (const { s, y: ly } of labels) {
      svg.append(el("svg:text", { class: "end-label", x: x(n - 1) + 8, y: ly + 4 },
        `${s.short ?? s.name} ${yFormat(s.values[n - 1])}`));
    }
  }

  const hair = el("svg:line", { class: "hair", y1: pad.top, y2: pad.top + innerH, visibility: "hidden" });
  const dots = series.map((s) => el("svg:circle", { class: "dot", r: 4, fill: cssVar(s.color), visibility: "hidden" }));
  svg.append(hair, ...dots);

  const tooltip = $("tooltip");
  let current = -1;

  function show(i, clientX, clientY) {
    current = i;
    const px = x(i);
    hair.setAttribute("x1", px); hair.setAttribute("x2", px); hair.setAttribute("visibility", "visible");
    series.forEach((s, k) => {
      dots[k].setAttribute("cx", px); dots[k].setAttribute("cy", y(s.values[i])); dots[k].setAttribute("visibility", "visible");
    });

    tooltip.replaceChildren(el("div", { class: "tt-date" }, fmtDate(times[i])));
    for (const s of series) {
      const row = el("div", { class: "tt-row" });
      const name = el("span", { class: "tt-name" });
      name.append(el("span", { class: "swatch", style: `background:${cssVar(s.color)}` }), document.createTextNode(s.name));
      row.append(el("strong", {}, yFormat(s.values[i])), name);
      tooltip.append(row);
    }
    tooltip.hidden = false;
    const box = svg.getBoundingClientRect();
    const cx = clientX ?? box.left + px;
    const cy = clientY ?? box.top + pad.top;
    const tw = tooltip.offsetWidth;
    tooltip.style.left = Math.min(Math.max(8, cx + 14), window.innerWidth - tw - 8) + "px";
    tooltip.style.top = Math.max(8, cy - tooltip.offsetHeight - 12) + "px";
  }

  function hide() {
    current = -1;
    hair.setAttribute("visibility", "hidden");
    dots.forEach((d) => d.setAttribute("visibility", "hidden"));
    tooltip.hidden = true;
  }

  svg.addEventListener("pointermove", (e) => {
    const box = svg.getBoundingClientRect();
    const px = ((e.clientX - box.left) / box.width) * width;
    const i = Math.round(((px - pad.left) / innerW) * (n - 1));
    if (i < 0 || i >= n) return hide();
    show(i, e.clientX, e.clientY);
  });
  svg.addEventListener("pointerleave", hide);
  svg.addEventListener("blur", hide);
  svg.addEventListener("keydown", (e) => {
    const step = { ArrowLeft: -1, ArrowRight: 1, Home: -n, End: n }[e.key];
    if (step === undefined) return;
    e.preventDefault();
    show(Math.min(n - 1, Math.max(0, (current < 0 ? n - 1 : current) + step)));
  });

  container.append(svg);
}

// ── page ─────────────────────────────────────────────────────────────────────

function renderTiles(a) {
  $("t-price").textContent = fmtUSD(a.current_price);
  const sig = $("t-signal");
  const kind = a.signal.includes("BUY") ? "good" : a.signal.includes("SELL") ? "critical" : "neutral";
  sig.className = `tile-value signal-${kind}`;
  sig.replaceChildren(
    el("span", { class: "signal-icon", "aria-hidden": "true" }, kind === "good" ? "▲" : kind === "critical" ? "▼" : "●"),
    document.createTextNode(a.signal),
  );
  $("t-score").textContent = (a.composite_score >= 0 ? "+" : "−") + Math.abs(a.composite_score).toFixed(2);
  $("t-forecast").textContent = fmtPct(a.forecast_change_pct);
  $("t-r2").textContent = `R² ${a.forecast_r2.toFixed(2)} over the last 14 days`;
}

function renderTables(wf, times) {
  const body = $("strategies").querySelector("tbody");
  body.replaceChildren();
  for (const s of STRATEGIES) {
    const r = wf[s.key];
    const tr = el("tr");
    const name = el("td");
    name.append(el("span", { class: "swatch", style: `background:${cssVar(s.color)};margin-right:8px` }), document.createTextNode(s.name));
    tr.append(
      name,
      el("td", { class: r.total_return_pct >= 0 ? "up" : "down" }, fmtPct(r.total_return_pct)),
      el("td", {}, fmtNum(r.sharpe_ratio)),
      el("td", {}, fmtNum(r.sortino_ratio)),
      el("td", {}, fmtPct(-r.max_drawdown_pct)),
      el("td", {}, s.key === "buy_hold" ? "—" : String(r.total_trades)),
    );
    body.append(tr);
  }

  const folds = $("folds").querySelector("tbody");
  folds.replaceChildren();
  wf.folds.forEach((f, i) => {
    const tr = el("tr");
    tr.append(
      el("td", {}, String(i + 1)),
      el("td", {}, `${fmtDate(times[f.test_start])} – ${fmtDate(times[f.test_end - 1])}`),
      el("td", {}, `${f.chosen.entry >= 0 ? "+" : "−"}${Math.abs(f.chosen.entry).toFixed(1)} / ${f.chosen.exit >= 0 ? "+" : "−"}${Math.abs(f.chosen.exit).toFixed(1)}`),
      el("td", {}, fmtNum(f.train_sharpe)),
      el("td", { class: f.test_return_pct > 0 ? "up" : f.test_return_pct < 0 ? "down" : "" }, fmtPct(f.test_return_pct)),
    );
    folds.append(tr);
  });
}

function render() {
  const coin = state.data.coins.find((c) => c.coin_id === state.coin);
  const wf = coin.walk_forward;
  const oos = wf.oos_start;

  for (const b of $("coins").children) b.setAttribute("aria-pressed", String(b.dataset.coin === state.coin));
  renderTiles(coin.analysis);

  $("price-title").textContent = `${coinName(coin.coin_id)} price`;
  lineChart($("price-chart"), {
    times: coin.times,
    series: [{ name: coinName(coin.coin_id), color: "--series-1", values: coin.prices }],
    yFormat: fmtUSD,
    shadeFrom: oos,
  });

  const oosTimes = coin.times.slice(oos);
  const days = oosTimes.length;
  $("equity-note").textContent =
    `Last ${days} days (${fmtDate(oosTimes[0])} – ${fmtDate(oosTimes[days - 1])}), ${wf.folds.length} folds of ` +
    `${state.data.test_days} days, each tuned on the preceding ${state.data.train_days} days over ${state.data.grid_size} threshold pairs.`;

  const legend = $("equity-legend");
  legend.replaceChildren(...STRATEGIES.map((s) => {
    const item = el("span");
    item.append(el("span", { class: "swatch", style: `background:${cssVar(s.color)}` }), document.createTextNode(s.name));
    return item;
  }));

  lineChart($("equity-chart"), {
    times: oosTimes,
    series: STRATEGIES.map((s) => ({ name: s.name, short: s.short, color: s.color, values: wf[s.key].equity })),
    yFormat: fmtUSD,
    endLabels: true,
  });

  renderTables(wf, coin.times);
}

async function main() {
  initTheme();
  try {
    const res = await fetch("data.json", { cache: "no-cache" });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    state.data = await res.json();
  } catch (err) {
    $("status").textContent = `Could not load data.json (${err.message}).`;
    return;
  }

  const nav = $("coins");
  for (const c of state.data.coins) {
    const b = el("button", { type: "button", "data-coin": c.coin_id }, coinName(c.coin_id));
    b.addEventListener("click", () => { state.coin = c.coin_id; render(); });
    nav.append(b);
  }
  state.coin = state.data.coins[0].coin_id;
  $("generated").textContent = `Data generated ${new Date(state.data.generated_at).toUTCString()}.`;
  $("status").hidden = true;
  $("content").hidden = false;
  render();

  let raf = 0;
  new ResizeObserver(() => { cancelAnimationFrame(raf); raf = requestAnimationFrame(render); }).observe($("price-chart"));
  matchMedia("(prefers-color-scheme: dark)").addEventListener("change", render);
}

main();
