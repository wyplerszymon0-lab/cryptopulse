// Package backtest evaluates the Engine's composite score as a trading signal
// over historical data, with no lookahead.
//
// Trading rules:
//   - The score for day T is computed at its close using only prices[0..T].
//   - Orders execute at the close of day T+1 (realistic next-day fill).
//   - 0.1% commission per side (typical centralised-exchange spot fee).
//   - Long-only: enter when score >= Params.Entry, exit when score <= Params.Exit.
//   - Portfolio starts at $10,000; partial units are supported.
package backtest

import (
	"fmt"
	"math"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
)

const (
	// MinDataPoints is the minimum price series length required for a meaningful backtest.
	MinDataPoints = 60
	// WarmupPeriod is the number of days used exclusively to initialise indicators.
	// Trading begins on day WarmupPeriod (signals computed from prices[0..WarmupPeriod]).
	WarmupPeriod = 30
	// InitialCapital is the starting portfolio value in USD.
	InitialCapital = 10_000.0
	// CommissionRate is the per-trade fee as a fraction of notional (0.1%).
	CommissionRate = 0.001
	// PeriodsPerYear annualises daily Sharpe and Sortino. Crypto trades every day
	// of the year, so this is 365, not the 252 trading days of equity markets.
	PeriodsPerYear = 365
)

// Params are the score thresholds that drive the long-only strategy.
type Params struct {
	Entry float64 `json:"entry"` // buy when flat and score >= Entry
	Exit  float64 `json:"exit"`  // sell when long and score <= Exit
}

// DefaultParams reproduce the engine's signal labels: enter on BUY or better
// (score >= 0.2), exit on SELL or worse (score <= -0.2).
var DefaultParams = Params{Entry: 0.2, Exit: -0.2}

// Trade records a single completed round-trip.
type Trade struct {
	EntryIndex int     `json:"entry_idx"`
	ExitIndex  int     `json:"exit_idx"`
	EntryPrice float64 `json:"entry_price"`
	ExitPrice  float64 `json:"exit_price"`
	ReturnPct  float64 `json:"return_pct"`
	Winner     bool    `json:"winner"`
}

// Result contains the full backtesting report for a single coin.
type Result struct {
	CoinID string `json:"coin_id"`

	// Return metrics
	TotalReturn      float64 `json:"total_return_pct"`
	AnnualizedReturn float64 `json:"annualized_return_pct"`
	BuyHoldReturn    float64 `json:"buy_hold_return_pct"`

	// Risk-adjusted metrics
	SharpeRatio  float64 `json:"sharpe_ratio"`
	SortinoRatio float64 `json:"sortino_ratio"`
	MaxDrawdown  float64 `json:"max_drawdown_pct"`

	// Trade statistics
	WinRate      float64 `json:"win_rate_pct"`
	TotalTrades  int     `json:"total_trades"`
	ProfitFactor float64 `json:"profit_factor"`

	// Metadata
	DataPoints int       `json:"data_points"`
	Trades     []Trade   `json:"trades,omitempty"`
	Equity     []float64 `json:"equity,omitempty"` // portfolio value at each day's close
}

// Predictor is the interface required by Run; satisfied by *predictor.Engine.
type Predictor interface {
	PredictAt(i int) (predictor.Result, error)
	Prices() []float64
}

// Scores returns the lookahead-free composite score for every day. Days that
// cannot trade (the warmup period, the last day, or a failed prediction) are NaN.
func Scores(eng Predictor) []float64 {
	n := len(eng.Prices())
	scores := make([]float64, n)
	for i := range scores {
		scores[i] = math.NaN()
		if i < WarmupPeriod || i >= n-1 {
			continue
		}
		if r, err := eng.PredictAt(i); err == nil {
			scores[i] = r.Score
		}
	}
	return scores
}

// Run backtests the default thresholds over the engine's full price series.
func Run(eng Predictor) (Result, error) {
	return RunWithParams(eng, DefaultParams)
}

// RunWithParams backtests the given thresholds over the engine's full price series.
func RunWithParams(eng Predictor, p Params) (Result, error) {
	prices := eng.Prices()
	n := len(prices)
	if n < MinDataPoints {
		return Result{}, fmt.Errorf("backtest requires at least %d prices, have %d", MinDataPoints, n)
	}
	sim := simulate(prices, Scores(eng), p, 0, n, InitialCapital)
	res := summarize(sim, InitialCapital)
	// Buy-and-hold is measured from the end of the warmup period to avoid penalising
	// the strategy for the period it was not yet active.
	res.BuyHoldReturn = pctChange(prices[WarmupPeriod], prices[n-1])
	res.DataPoints = n
	return res, nil
}

// simulation is the raw outcome of trading one window.
type simulation struct {
	equity []float64 // portfolio value at the close of each day in the window
	trades []Trade
}

// simulate trades days [from, to) starting flat with `capital`. A decision on day
// i fills at the close of day i+1; any position still open is closed at the close
// of day to-1, so the window always ends in cash.
func simulate(prices, scores []float64, p Params, from, to int, capital float64) simulation {
	sim := simulation{equity: make([]float64, 0, to-from)}
	cash, holdings := capital, 0.0
	inPos := false
	var entryIdx int
	var entryPrice float64

	closePosition := func(idx int) {
		price := prices[idx]
		cash = holdings * price * (1 - CommissionRate)
		ret := pctChange(entryPrice, price)
		sim.trades = append(sim.trades, Trade{
			EntryIndex: entryIdx, ExitIndex: idx,
			EntryPrice: entryPrice, ExitPrice: price,
			ReturnPct: ret, Winner: ret > 0,
		})
		holdings, inPos = 0, false
	}

	for i := from; i < to; i++ {
		if i == to-1 && inPos {
			closePosition(i)
		}
		if inPos {
			sim.equity = append(sim.equity, holdings*prices[i])
		} else {
			sim.equity = append(sim.equity, cash)
		}

		if i >= to-1 || math.IsNaN(scores[i]) {
			continue
		}
		next := i + 1
		switch {
		case !inPos && scores[i] >= p.Entry:
			holdings = cash * (1 - CommissionRate) / prices[next]
			entryPrice, entryIdx = prices[next], next
			cash, inPos = 0, true
		case inPos && scores[i] <= p.Exit:
			closePosition(next)
		}
	}
	return sim
}

// summarize turns a simulation into performance metrics.
func summarize(sim simulation, capital float64) Result {
	eq := sim.equity
	final := eq[len(eq)-1]
	winRate, profitFactor := tradeMetrics(sim.trades)
	returns := dailyReturns(eq)
	return Result{
		TotalReturn:      pctChange(capital, final),
		AnnualizedReturn: annualizedReturn(final, capital, len(eq)),
		SharpeRatio:      sharpeRatio(returns),
		SortinoRatio:     sortinoRatio(returns),
		MaxDrawdown:      maxDrawdown(eq),
		WinRate:          winRate,
		TotalTrades:      len(sim.trades),
		ProfitFactor:     profitFactor,
		Trades:           sim.trades,
		Equity:           eq,
	}
}

func pctChange(from, to float64) float64 {
	return (to - from) / from * 100
}

func dailyReturns(equity []float64) []float64 {
	out := make([]float64, 0, len(equity))
	for i := 1; i < len(equity); i++ {
		if equity[i-1] > 0 {
			out = append(out, equity[i]/equity[i-1]-1)
		}
	}
	return out
}

func maxDrawdown(equity []float64) float64 {
	var peak, maxDD float64
	for _, v := range equity {
		peak = math.Max(peak, v)
		if peak > 0 {
			maxDD = math.Max(maxDD, (peak-v)/peak*100)
		}
	}
	return maxDD
}

func annualizedReturn(final, initial float64, days int) float64 {
	if initial <= 0 || final <= 0 || days <= 0 {
		return 0
	}
	years := float64(days) / 365.0
	return (math.Pow(final/initial, 1/years) - 1) * 100
}

// sharpeRatio returns the annualised Sharpe ratio assuming a zero risk-free rate,
// which is standard practice in crypto backtesting.
func sharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	m := mean(returns)
	s := stdDev(returns, m)
	if s == 0 {
		return 0
	}
	return m / s * math.Sqrt(PeriodsPerYear)
}

// sortinoRatio is like Sharpe but penalises only downside deviation:
// sqrt(mean(min(r, 0)^2)) over *all* periods, with a zero target return.
func sortinoRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	var sumSq float64
	for _, r := range returns {
		if r < 0 {
			sumSq += r * r
		}
	}
	downside := math.Sqrt(sumSq / float64(len(returns)))
	if downside == 0 {
		return 0
	}
	return mean(returns) / downside * math.Sqrt(PeriodsPerYear)
}

func tradeMetrics(trades []Trade) (winRate, profitFactor float64) {
	if len(trades) == 0 {
		return 0, 0
	}
	var wins int
	var grossProfit, grossLoss float64
	for _, t := range trades {
		if t.Winner {
			wins++
			grossProfit += t.ReturnPct
		} else {
			grossLoss += math.Abs(t.ReturnPct)
		}
	}
	winRate = float64(wins) / float64(len(trades)) * 100
	if grossLoss > 0 {
		profitFactor = grossProfit / grossLoss
	}
	return winRate, profitFactor
}

func mean(xs []float64) float64 {
	var s float64
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func stdDev(xs []float64, m float64) float64 {
	var s float64
	for _, x := range xs {
		d := x - m
		s += d * d
	}
	return math.Sqrt(s / float64(len(xs)))
}
