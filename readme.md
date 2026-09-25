# CryptoPulse

**Real-time cryptocurrency technical analysis and walk-forward backtesting engine written in Go.**

[![CI](https://github.com/wyplerszymon0-lab/cryptopulse/actions/workflows/ci.yml/badge.svg)](https://github.com/wyplerszymon0-lab/cryptopulse/actions)
![Go 1.22](https://img.shields.io/badge/Go-1.22-00ADD8?logo=go)
![Zero Dependencies](https://img.shields.io/badge/dependencies-zero-brightgreen)
![License: MIT](https://img.shields.io/badge/license-MIT-blue)

---

## What it does

CryptoPulse fetches live price data from the CoinGecko public API, computes **7 technical indicators** in a weighted composite model, and generates a directional signal with a confidence score. It also ships a **walk-forward backtesting engine** that replays signals over historical data without lookahead bias, producing professional performance metrics: Sharpe ratio, Sortino ratio, maximum drawdown, win rate, and profit factor — compared against a buy-and-hold baseline.

---

## Demo

```
╔══════════════════════════════════════════════════════════╗
║  CryptoPulse v2.0.0  ·  Technical Analysis & Backtesting  ║
║  Go  ·  Zero Dependencies  ·  7 Indicators  ·  Backtest   ║
╚══════════════════════════════════════════════════════════╝

Fetched 3 coin(s) in 1.24s

━━━━━━━━━━━━━━━━━ BITCOIN ━━━━━━━━━━━━━━━━

  Current Price           $67,420.00
  Forecast (1-day)        $68,150.32    +1.08%    R² 0.94

  Signal  ►  STRONG BUY              Score  +0.742

  ─ Indicators ──────────────────────────────────
  RSI (14)                  28.4    Oversold
  MACD Line              +124.32    ▲ Bullish (hist +18.2)
  Bollinger Position        18.4%   Near lower  (mean-reversion)
  SMA 7 / 20            67.1k / 62.8k  Golden Cross  ▲
  ATR (14)                $2,341    moderate volatility (3.5%/day)
  Stochastic (14,3)     K:23.1  D:31.4  Oversold

  ─ Risk Profile ────────────────────────────────
  Confidence              74.2%   ████████████░░░░░░░░
```

### Backtest output (`--backtest --days 365`)

```
━━━━━━━━━━━━━━━━━━━━━━━━━ BACKTEST RESULTS ━━━━━━━━━━━━━━━━━━━━━━━━━

  Coin        Total Ret   Ann. Ret   Sharpe   Sortino   Max DD   Win Rate  Trades   vs B&H
  ──────────────────────────────────────────────────────────────────────────────────────────
  BITCOIN      +62.4%      +62.4%     1.84     2.31    -12.3%     63.2%     18     -34.2%p
  ETHEREUM     +48.1%      +48.1%     1.62     2.04    -18.7%     58.9%     22      +3.1%p
  SOLANA      +124.7%     +124.7%     2.14     2.87    -24.1%     61.7%     14    -212.3%p

  Trades fill at next-day close · 0.1% commission per side · long-only
  vs B&H: percentage-point difference vs buying and holding from warmup day
  WARNING: Past performance does not predict future results.
```

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

- **`PredictAt(i int)`** — the single lookahead barrier. The backtest engine calls this to compute signals on `prices[0..i]`, guaranteeing that no future price ever influences a historical signal.
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

- **Walk-forward, no lookahead**: signals at day *T* are computed exclusively from `prices[0..T]`.
- **Execution model**: orders fill at the *next* day's closing price, simulating realistic end-of-day workflow.
- **Commission**: 0.1% per side (typical centralised-exchange spot fee).
- **Position sizing**: 100% of available capital per trade (long-only, no leverage).
- **Warmup**: the first 30 days initialise indicators; trading begins on day 31.
- **Sharpe ratio**: annualised, zero risk-free rate (standard for crypto).
- **Sortino ratio**: like Sharpe but penalises only downside deviation.
- **Max drawdown**: peak-to-trough portfolio decline over the full simulation.
- **Profit factor**: gross profit ÷ gross loss across all completed trades.

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

# 1-year walk-forward backtest
go run . --backtest --days 365

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
  --coins     comma-separated CoinGecko IDs  (default "bitcoin,ethereum,solana")
  --days      historical days to fetch, 30-365  (default 90)
  --backtest  run walk-forward backtest instead of live analysis
  --json      output as JSON (pipe-friendly)
  --workers   concurrent fetch goroutines  (default 3)
  --verbose   enable debug logging
  --version   print version and exit
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
    │   ├── backtest.go            Walk-forward engine: Sharpe, Sortino, drawdown, PnL
    │   └── backtest_test.go       Metric validity, buy-hold baseline, trade accounting
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
