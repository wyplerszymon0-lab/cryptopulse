// Package backtest provides a walk-forward backtesting engine for evaluating
// the Engine's signal quality over historical data.
//
// Trading rules:
//   - Signal computed at close of day T using only prices[0..T] (no lookahead).
//   - Order executes at close of day T+1 (realistic next-day fill).
//   - 0.1% commission per side (typical centralised-exchange spot fee).
//   - Long-only strategy: enter on BUY/STRONG BUY, exit on SELL/STRONG SELL.
//   - Portfolio starts at $10,000; partial shares are supported.
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
)

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
	DataPoints int     `json:"data_points"`
	Trades     []Trade `json:"trades,omitempty"`
}

// Predictor is the interface required by Run; satisfied by *predictor.Engine.
type Predictor interface {
	PredictAt(i int) (predictor.Result, error)
	Prices() []float64
}

// Run simulates the strategy over the engine's price series and returns performance metrics.
func Run(eng Predictor) (Result, error) {
	prices := eng.Prices()
	n := len(prices)
	if n < MinDataPoints {
		return Result{}, fmt.Errorf("backtest requires at least %d prices, have %d", MinDataPoints, n)
	}

	// portfolioValues[i] = portfolio value (mark-to-market) at close of day i.
	portfolioValues := make([]float64, n)

	var (
		cash       = InitialCapital
		holdings   float64
		inPos      bool
		entryIdx   int
		entryPrice float64
		trades     []Trade
		peakValue  = InitialCapital
		maxDD      float64
	)

	for i := 0; i < n; i++ {
		// Mark portfolio to market at today's close.
		if inPos {
			portfolioValues[i] = holdings * prices[i]
		} else {
			portfolioValues[i] = cash
		}

		// Track running peak and compute drawdown.
		if portfolioValues[i] > peakValue {
			peakValue = portfolioValues[i]
		}
		if peakValue > 0 {
			dd := (peakValue - portfolioValues[i]) / peakValue * 100
			if dd > maxDD {
				maxDD = dd
			}
		}

		// No trading during warmup or on the final day (no next-day fill possible).
		if i < WarmupPeriod || i >= n-1 {
			continue
		}

		// Compute signal on all history available through today (no lookahead).
		result, err := eng.PredictAt(i)
		if err != nil {
			continue // skip day if prediction fails (e.g. not enough warmup yet)
		}

		nextPrice := prices[i+1]

		switch {
		case !inPos && result.Signal >= predictor.Buy:
			// Enter long: buy at tomorrow's close.
			holdings = cash * (1 - CommissionRate) / nextPrice
			entryPrice = nextPrice
			entryIdx = i + 1
			cash = 0
			inPos = true

		case inPos && result.Signal <= predictor.Sell:
			// Exit long: sell at tomorrow's close.
			proceeds := holdings * nextPrice * (1 - CommissionRate)
			ret := (nextPrice - entryPrice) / entryPrice * 100
			trades = append(trades, Trade{
				EntryIndex: entryIdx,
				ExitIndex:  i + 1,
				EntryPrice: entryPrice,
				ExitPrice:  nextPrice,
				ReturnPct:  ret,
				Winner:     ret > 0,
			})
			cash = proceeds
			holdings = 0
			inPos = false
		}
	}

	// Force-close any open position at the last price.
	if inPos {
		last := prices[n-1]
		proceeds := holdings * last * (1 - CommissionRate)
		ret := (last - entryPrice) / entryPrice * 100
		trades = append(trades, Trade{
			EntryIndex: entryIdx,
			ExitIndex:  n - 1,
			EntryPrice: entryPrice,
			ExitPrice:  last,
			ReturnPct:  ret,
			Winner:     ret > 0,
		})
		cash = proceeds
		portfolioValues[n-1] = cash
	}

	// Compute daily returns from the portfolio value series.
	dailyReturns := make([]float64, 0, n-1)
	for i := 1; i < n; i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	finalValue := cash
	totalRet := (finalValue - InitialCapital) / InitialCapital * 100
	annRet := annualizedReturn(finalValue, InitialCapital, n)
	sharpe := sharpeRatio(dailyReturns)
	sortino := sortinoRatio(dailyReturns)
	winRate, profitFactor := tradeMetrics(trades)
	// Buy-and-hold is measured from the end of the warmup period to avoid penalising
	// the strategy for the period it was not yet active.
	buyHold := (prices[n-1] - prices[WarmupPeriod]) / prices[WarmupPeriod] * 100

	return Result{
		TotalReturn:      totalRet,
		AnnualizedReturn: annRet,
		BuyHoldReturn:    buyHold,
		SharpeRatio:      sharpe,
		SortinoRatio:     sortino,
		MaxDrawdown:      maxDD,
		WinRate:          winRate,
		TotalTrades:      len(trades),
		ProfitFactor:     profitFactor,
		DataPoints:       n,
		Trades:           trades,
	}, nil
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
	return m / s * math.Sqrt(252)
}

// sortinoRatio is like Sharpe but penalises only downside deviation.
func sortinoRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	m := mean(returns)

	var neg []float64
	for _, r := range returns {
		if r < 0 {
			neg = append(neg, r)
		}
	}
	if len(neg) == 0 {
		return 0
	}
	ds := stdDev(neg, 0)
	if ds == 0 {
		return 0
	}
	return m / ds * math.Sqrt(252)
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
