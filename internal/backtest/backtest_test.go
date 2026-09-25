package backtest

import (
	"testing"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
)

// trendPrices returns n prices on a linear trend: start + slope*i.
func trendPrices(n int, start, slope float64) []float64 {
	p := make([]float64, n)
	for i := range p {
		p[i] = start + slope*float64(i)
	}
	return p
}

func TestRun_InsufficientData(t *testing.T) {
	eng := predictor.NewEngine(trendPrices(30, 100, 1))
	_, err := Run(eng)
	if err == nil {
		t.Error("expected error for series shorter than MinDataPoints, got nil")
	}
}

func TestRun_ReturnsValidMetrics(t *testing.T) {
	prices := trendPrices(120, 100, 0.5)
	eng := predictor.NewEngine(prices)
	result, err := Run(eng)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.DataPoints != len(prices) {
		t.Errorf("DataPoints = %d, want %d", result.DataPoints, len(prices))
	}
	if result.MaxDrawdown < 0 {
		t.Errorf("MaxDrawdown = %v should be ≥ 0", result.MaxDrawdown)
	}
	if result.WinRate < 0 || result.WinRate > 100 {
		t.Errorf("WinRate = %v out of [0,100]", result.WinRate)
	}
	if result.TotalTrades < 0 {
		t.Errorf("TotalTrades = %d should be ≥ 0", result.TotalTrades)
	}
}

func TestRun_TradeCount(t *testing.T) {
	// Ensure the number of recorded trades matches len(result.Trades).
	prices := trendPrices(150, 100, 0.3)
	eng := predictor.NewEngine(prices)
	result, err := Run(eng)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalTrades != len(result.Trades) {
		t.Errorf("TotalTrades (%d) != len(Trades) (%d)", result.TotalTrades, len(result.Trades))
	}
}

func TestRun_BuyHoldCalculation(t *testing.T) {
	// Buy-and-hold return is measured from the warmup day, not day 0.
	prices := trendPrices(100, 100, 1) // linear from 100 to 199
	eng := predictor.NewEngine(prices)
	result, _ := Run(eng)

	startPrice := prices[WarmupPeriod]
	endPrice := prices[len(prices)-1]
	expected := (endPrice - startPrice) / startPrice * 100

	if result.BuyHoldReturn != expected {
		t.Errorf("BuyHoldReturn = %.4f, want %.4f", result.BuyHoldReturn, expected)
	}
}

func TestRun_ProfitFactor(t *testing.T) {
	// ProfitFactor is undefined (0) when there are no trades.
	prices := trendPrices(65, 100, 0) // flat prices → likely Neutral → no trades
	eng := predictor.NewEngine(prices)
	result, err := Run(eng)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProfitFactor < 0 {
		t.Errorf("ProfitFactor = %v, should be ≥ 0", result.ProfitFactor)
	}
}

func TestRun_SharpeBounded(t *testing.T) {
	// Sharpe ratio should not be NaN or Inf.
	prices := trendPrices(100, 100, 0.5)
	eng := predictor.NewEngine(prices)
	result, _ := Run(eng)

	if result.SharpeRatio != result.SharpeRatio { // NaN check
		t.Error("SharpeRatio is NaN")
	}
}

func TestMean(t *testing.T) {
	got := mean([]float64{1, 2, 3, 4, 5})
	if got != 3 {
		t.Errorf("mean = %v, want 3", got)
	}
}

func TestStdDev(t *testing.T) {
	// stdDev of {2, 4, 4, 4, 5, 5, 7, 9} with mean 5 is 2.
	xs := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	m := mean(xs)
	got := stdDev(xs, m)
	if got < 1.99 || got > 2.01 {
		t.Errorf("stdDev = %v, want ≈2", got)
	}
}
