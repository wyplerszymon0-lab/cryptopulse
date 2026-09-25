# CryptoPulse

**Real-time cryptocurrency technical analysis and walk-forward backtesting engine written in Go.**

[![CI](https://github.com/wyplerszymon0-lab/cryptopulse/actions/workflows/ci.yml/badge.svg)](https://github.com/wyplerszymon0-lab/cryptopulse/actions)
![Go 1.22](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)
![Zero Dependencies](https://img.shields.io/badge/dependencies-zero-brightgreen)
![License: MIT](https://img.shields.io/badge/license-MIT-blue)

**[Live dashboard →](https://wyplerszymon0-lab.github.io/cryptopulse/)** rebuilt every day by GitHub Actions: today's signals, price history and out-of-sample equity curves for BTC, ETH and SOL.

---

## What it does

CryptoPulse fetches daily prices from the CoinGecko public API, combines **5 technical indicators** into a weighted composite score, and turns it into a directional signal. On top of that it ships:

- a **lookahead-free backtester** (signal at day *T* uses only prices up to *T*, orders fill at *T+1*, 0.1% commission), reporting Sharpe, Sortino, max drawdown, win rate and profit factor against buy-and-hold;
- **walk-forward optimisation**: entry/exit thresholds are tuned on a rolling training window and scored only on the following unseen window, so every reported number is out-of-sample;
- a **static web dashboard**, regenerated daily from the CLI's `--export` output and deployed to GitHub Pages.
---

## Demo

Real output (captured 25 Sep 2026). Numbers change daily with the market.

### Live analysis (`--coins bitcoin`)

```
Fetched 1 coin(s) in 307ms

━━━━━━━━━━━━━━━━━━━━━━ BITCOIN ━━━━━━━━━━━━━━━━━━━━━━

  Current Price           $83,879.65
  Forecast (1-day)        $86,683.86   +3.34%   R² 0.73

  Signal  ►  BUY            Score +0.289

  ─ Indicators ──────────────────────────────────
  RSI (14)                    63.6   Neutral
  MACD Line                 +2470.11   ▲ Bullish (hist +376.88)
  Bollinger Position         78.8%   Mid-band
  SMA 7 / 20                84.0k / 79.9k  Golden Cross  ▲
  ATR (14)                  $1,225.98  low volatility (1.5%/day)
  Stochastic (14,3)         K:75.3  D:78.3   Neutral

  ─ Risk Profile ────────────────────────────────
  Confidence                28.9%   ██████░░░░░░░░░░░░░░
```

### Backtest of the default thresholds (`--backtest --days 365`)

```
━━━━━━━━━━━━━━━━━━━━━━━━━━ BACKTEST RESULTS ━━━━━━━━━━━━━━━━━━━━━━━━━━

  Coin        Total Ret   Ann. Ret    Sharpe    Sortino    Max DD     Win Rate   Trades   vs B&H    
  ──────────────────────────────────────────────────────────────────────────────────────────────
  BITCOIN         -3.7%      -3.6%     -0.04      -0.06     -14.6%      25.0%       12     +21.2%p
  ETHEREUM       +11.9%     +11.8%      0.49       0.74     -19.8%      54.5%       11     +43.9%p
  SOLANA          -0.8%      -0.8%      0.19       0.28     -29.9%      30.0%       10     +36.6%p

  3  coin(s) backtested
  Trades fill at next-day close · 0.1% commission per side · long-only
  vs B&H: percentage-point difference vs buying and holding from warmup day

  WARNING: Past performance does not predict future results.
```

### Walk-forward optimisation (`--optimize --coins ethereum --days 365`)

```
━━━━━━━━━━━━━━━━━━━━━ WALK-FORWARD OPTIMISATION ━━━━━━━━━━━━━━━━━━━━━

  ETHEREUM  out-of-sample: last 216 days, 8 folds

  Strategy                 Total Ret    Sharpe    Sortino    Max DD   Trades
  ────────────────────────────────────────────────────────────────────────
  Walk-forward optimised      +27.5%      1.40       2.62    -17.9%        8
  Fixed default (±0.2)        +25.9%      1.24       2.13    -19.8%        8
  Buy & hold                  +42.4%      1.34       2.28    -35.3%        0

  Fold  Test days   Chosen entry/exit   Train Sharpe   Test return
     1  150–179      +0.1 / -0.3           -0.36         +15.5%
     2  180–209      +0.5 / -0.1            1.59          +0.0%
     3  210–239      +0.2 / -0.3            1.42          -8.7%
     4  240–269      +0.1 / +0.0            1.53          +4.3%
     5  270–299      +0.1 / +0.0            2.12         +15.9%
     6  300–329      +0.5 / -0.5            3.17          +0.0%
     7  330–359      +0.5 / -0.5            3.23          +0.0%
     8  360–365      +0.5 / -0.5            3.57          +0.0%

  Parameters are chosen by in-sample Sharpe on each training window and only
  scored on the following test window, so every number above is out-of-sample.
  WARNING: Past performance does not predict future results.
```

## What the walk-forward results say

Across BTC, ETH and SOL over the same 216 out-of-sample days (Feb–Sep 2026):

| Coin | Optimised | Fixed ±0.2 | Buy & hold | Max drawdown (optimised / fixed / B&H) |
|---|---:|---:|---:|---|
| BTC | −4.8% | +5.3% | +28.6% | −8.3% / −14.6% / −28.6% |
| ETH | +27.5% | +25.9% | +42.4% | −17.9% / −19.8% / −35.3% |
| SOL | +5.2% | +21.5% | +54.1% | −18.6% / −23.2% / −36.1% |

- **Tuning the thresholds does not help.** The in-sample winner beats the fixed default on only one coin out of three: the best parameters for the last 120 days mostly do not carry over to the next 30. That is overfitting, and it only shows up because the evaluation is out-of-sample.
- **In a rising market the signal lags buy-and-hold on return, but roughly halves the drawdown.** Over the full year, which included a ~25% BTC decline, the same fixed strategy beat buy-and-hold by 21–44 percentage points by staying out of the fall (see the backtest table above).
---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  CLI (main.go)                                                  │
│  flag parsing · worker pool · JSON / terminal dispatch          │
└───────────────────────────────┬─────────────────────────────────┘
                                │
          ┌─────────────────────┼─────────────────────┐
          ▼                     ▼                     ▼
  ┌───────────────┐   ┌─────────────────┐   ┌─────────────────┐
  │  api/         │   │  predictor/     │   │  backtest/      │
  │  CoinGecko    │   │  Engine         │   │  Run()          │
  │  HTTP client  │   │  Predict()      │   │  walk-forward   │
  │  retry/backof │   │  PredictAt(i)   │   │  Sharpe/Sortino │
  └───────────────┘   └────────┬────────┘   └────────┬────────┘
                               │                     │
                               ▼                     │
                    ┌─────────────────┐              │
                    │  indicators/    │◄─────────────┘
                    │  SMA · EMA      │
                    │  RSI · MACD     │
                    │  Bollinger      │
                    │  ATR · Stoch    │
                    │  LinRegression  │
                    └─────────────────┘
```

**Key design choices:**

- **`PredictAt(i int)`** — the single lookahead barrier. `backtest.Scores` calls it once per day on `prices[0..i]`, guaranteeing that no future price ever influences a historical signal; every simulation and every walk-forward fold then reuses those scores.
- **Simulation is separate from metrics.** `simulate` trades any window `[from, to)` with any thresholds and always ends in cash; `summarize` turns the equity curve into metrics. The plain backtest, each training window and each test fold are all the same function.
- **Graceful indicator degradation** — the composite score is computed over whichever indicators have sufficient data at each time step. Early in a backtest (days 30–34), MACD may not be ready; the normalised score still reflects the available indicators rather than erroring.
- **`emaFull()`** — the fixed internal EMA helper returns a full-length series aligned to the input price index. The original version returned a trimmed slice, causing a silent mis-indexing bug in MACD reconstruction that produced wrong signal values without any error.

---

## Technical Indicators

| Indicator | Period | Weight | Signal Logic |
|-----------|--------|--------|--------------|
| RSI | 14 | 25% | ≤30 oversold (+), ≥70 overbought (−) |
| MACD | 12/26/9 | 25% | Histogram + line direction |
| Bollinger Bands | 20, 2σ | 15% | Mean-reversion: lower band (+), upper band (−) |
| SMA Crossover | 7/20 | 20% | Golden cross (+), death cross (−) |
| Stochastic | 14/3 | 15% | %K/%D in oversold/overbought zones |
| ATR | 14 | — | Informational: daily volatility sizing |
| Linear Regression | 14-day | — | 1-day forecast with R² confidence |

Composite score is normalised over the weights of indicators that could be computed. Score maps to signals: `≥0.6 STRONG BUY`, `≥0.2 BUY`, `±0.2 NEUTRAL`, `≤-0.2 SELL`, `≤-0.6 STRONG SELL`.

---

## Backtesting Methodology

- **No lookahead**: signals at day *T* are computed exclusively from `prices[0..T]`. A test changes prices after a fold and checks that the fold's chosen parameters and result are unchanged.
- **Execution model**: orders fill at the *next* day's closing price, simulating realistic end-of-day workflow.
- **Commission**: 0.1% per side (typical centralised-exchange spot fee).
- **Position sizing**: 100% of available capital per trade (long-only, no leverage).
- **Warmup**: the first 30 days initialise indicators; trading begins on day 31.
- **Strategy**: enter when the composite score is ≥ `entry`, exit when it is ≤ `exit`. The defaults (+0.2 / −0.2) match the BUY and SELL signal labels.
- **Walk-forward optimisation**: after warmup, a 120-day training window picks the (entry, exit) pair with the best Sharpe ratio from a 29-point grid; the next 30 days are traded with it; then the window rolls forward 30 days. Test windows start flat and end in cash, and their results are compounded.
- **Sharpe ratio**: annualised with √365 (crypto trades every day), zero risk-free rate.
- **Sortino ratio**: mean return over downside deviation, √(mean(min(r, 0)²)) across *all* days, annualised with √365.
- **Max drawdown**: peak-to-trough portfolio decline over the simulation.
- **Profit factor**: sum of winning trade returns ÷ sum of losing trade returns.
---

## Quick Start

```bash
# Prerequisites: Go 1.22+, internet connection (CoinGecko public API, no key needed)
git clone https://github.com/wyplerszymon0-lab/cryptopulse
cd cryptopulse

# Live analysis: BTC, ETH, SOL (default)
go run .

# Custom coins
go run . --coins bitcoin,ethereum,cardano,dogecoin

# 1-year backtest of the default thresholds
go run . --backtest --days 365

# Walk-forward optimisation (tune on 120 days, trade the next 30, repeat)
go run . --optimize --days 365

# Dashboard: write web/data.json and serve web/ on http://localhost:8000
make dashboard

# JSON output (pipe to jq, persist to file, etc.)
go run . --json | jq '.[0] | {signal, score: .composite_score}'

# Docker
make docker
docker run --rm cryptopulse:latest --coins bitcoin --days 90
```

---

## CLI Reference

```
Usage:
  predictor [flags]

Flags:
  --coins       comma-separated CoinGecko IDs  (default "bitcoin,ethereum,solana")
  --days        historical days to fetch, 30-365  (default 90)
  --backtest    backtest the default thresholds over the whole period
  --optimize    walk-forward optimisation: tune in-sample, score out-of-sample
  --train-days  walk-forward training window  (default 120)
  --test-days   walk-forward test window  (default 30)
  --export      write analysis + walk-forward results with price history to a JSON file
  --json        output as JSON (pipe-friendly)
  --workers     concurrent fetch goroutines  (default 3)
  --verbose     enable debug logging
  --version     print version and exit
```
---

## Project Structure

```
.
├── main.go                        Entry point — CLI, concurrent fetch, dispatch
├── go.mod                         Module definition (zero external dependencies)
├── Makefile                       build / test / bench / docker targets
├── Dockerfile                     Multi-stage build → ~5 MB scratch image
├── .github/workflows/ci.yml       CI: vet + race-detector tests + Docker build
├── .github/workflows/pages.yml    Daily: regenerate data and deploy the dashboard
├── web/                           Static dashboard (HTML/CSS/JS, no build step)
└── internal/
    ├── api/
    │   └── coingecko.go           HTTP client with retry + exponential backoff
    ├── indicators/
    │   ├── indicators.go          SMA, EMA, RSI, MACD, Bollinger, ATR, Stoch, LinReg
    │   └── indicators_test.go     Table-driven tests, error-path coverage
    ├── predictor/
    │   ├── engine.go              Composite scoring engine, PredictAt lookahead barrier
    │   └── engine_test.go         Signal ordering invariants, PredictAt consistency
    ├── backtest/
    │   ├── backtest.go            Lookahead-free simulation + metrics (Sharpe, Sortino, drawdown)
    │   ├── walkforward.go         Rolling optimise-on-train, score-on-test
    │   └── *_test.go              Fills, commission, metrics, fold tiling, no-lookahead check
    └── report/
        └── report.go              Coloured terminal output + structured JSON
```

---

## Development

```bash
make build      # compile binary
make test       # run tests with race detector
make bench      # run benchmarks
make vet        # static analysis
make docker     # build Docker image
make backtest   # 1-year walk-forward backtest (requires internet)
```

---

## Disclaimer

This project is for **educational and research purposes only**. It is not financial advice. Cryptocurrency markets are highly volatile. Past backtest performance does not predict future results. Never make investment decisions based solely on automated signals.

---

## License

MIT — see [LICENSE](LICENSE).
