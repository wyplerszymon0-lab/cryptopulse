package predictor

import (
	"fmt"
	"math"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/indicators"
)

// Signal represents a directional market signal as an ordered integer,
// allowing direct comparison (e.g. signal >= Buy).
type Signal int

const (
	StrongSell Signal = -2
	Sell        Signal = -1
	Neutral     Signal = 0
	Buy         Signal = 1
	StrongBuy   Signal = 2
)

func (s Signal) String() string {
	switch s {
	case StrongBuy:
		return "STRONG BUY"
	case Buy:
		return "BUY"
	case Neutral:
		return "NEUTRAL"
	case Sell:
		return "SELL"
	case StrongSell:
		return "STRONG SELL"
	default:
		return "UNKNOWN"
	}
}

// MarshalJSON serializes Signal as its human-readable string label.
func (s Signal) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// IndicatorSnapshot captures all computed indicator values at a single point in time.
type IndicatorSnapshot struct {
	RSI        float64 `json:"rsi_14"`
	MACDLine   float64 `json:"macd_line"`
	MACDSignal float64 `json:"macd_signal"`
	MACDHist   float64 `json:"macd_histogram"`
	BBUpper    float64 `json:"bb_upper"`
	BBMiddle   float64 `json:"bb_middle"`
	BBLower    float64 `json:"bb_lower"`
	BBPosition float64 `json:"bb_position"`
	SMAFast    float64 `json:"sma_7"`
	SMASlow    float64 `json:"sma_20"`
	ATR        float64 `json:"atr_14"`
	StochK     float64 `json:"stoch_k"`
	StochD     float64 `json:"stoch_d"`
}

// Result holds a complete analysis result for a single coin.
type Result struct {
	CoinID        string            `json:"coin_id"`
	CurrentPrice  float64           `json:"current_price"`
	Forecast      float64           `json:"forecast_price"`
	ForecastDelta float64           `json:"forecast_change_pct"`
	ForecastR2    float64           `json:"forecast_r2"`
	Signal        Signal            `json:"signal"`
	Score         float64           `json:"composite_score"`
	Confidence    float64           `json:"confidence_pct"`
	Indicators    IndicatorSnapshot `json:"indicators"`
}

// Engine holds a closing-price series and produces predictions from it.
type Engine struct {
	prices []float64
}

// NewEngine creates an Engine for the given daily closing price series.
func NewEngine(prices []float64) *Engine {
	return &Engine{prices: prices}
}

// Prices returns the full price series.
func (e *Engine) Prices() []float64 {
	return e.prices
}

// PredictAt computes a prediction using only prices[0..i] (inclusive).
// This is the lookahead-free entry point consumed by the backtesting engine;
// it guarantees that no future price information influences the signal.
func (e *Engine) PredictAt(i int) (Result, error) {
	if i < 0 || i >= len(e.prices) {
		return Result{}, fmt.Errorf("index %d out of range [0, %d)", i, len(e.prices))
	}
	sub := &Engine{prices: e.prices[:i+1]}
	return sub.Predict()
}

// Predict generates a signal and next-day forecast from the full price series.
// It requires at least 30 prices to initialise all indicators.
func (e *Engine) Predict() (Result, error) {
	prices := e.prices
	if len(prices) < 30 {
		return Result{}, fmt.Errorf("need at least 30 prices, have %d", len(prices))
	}
	current := prices[len(prices)-1]

	snap, score, err := computeIndicators(prices)
	if err != nil {
		return Result{}, fmt.Errorf("indicators: %w", err)
	}

	// 14-day regression window balances responsiveness with noise suppression.
	regWindow := min(14, len(prices))
	reg, err := indicators.LinearRegression(prices[len(prices)-regWindow:])
	if err != nil {
		return Result{}, fmt.Errorf("regression: %w", err)
	}

	delta := (reg.Forecast - current) / current * 100
	sig := scoreToSignal(score)

	return Result{
		CurrentPrice:  current,
		Forecast:      reg.Forecast,
		ForecastDelta: delta,
		ForecastR2:    reg.R2,
		Signal:        sig,
		Score:         score,
		Confidence:    math.Min(math.Abs(score)*100, 100),
		Indicators:    snap,
	}, nil
}

// computeIndicators runs all indicators and returns a snapshot and the normalised composite score.
// Indicators that lack sufficient data are silently skipped; the score is normalised to [-1, 1]
// over whichever subset could be computed.
func computeIndicators(prices []float64) (IndicatorSnapshot, float64, error) {
	var snap IndicatorSnapshot
	var score, totalWeight float64

	// RSI — weight 0.25
	if rsi, err := indicators.RSI(prices, 14); err == nil {
		snap.RSI = rsi
		score += rsiScore(rsi) * 0.25
		totalWeight += 0.25
	}

	// MACD — weight 0.25
	if macd, err := indicators.MACD(prices); err == nil {
		snap.MACDLine = macd.Line
		snap.MACDSignal = macd.Signal
		snap.MACDHist = macd.Histogram
		score += macdScore(macd.Line, macd.Histogram) * 0.25
		totalWeight += 0.25
	}

	// Bollinger Bands — weight 0.15
	// Price near the lower band signals a potential mean-reversion bounce (bullish);
	// near the upper band signals overextension (bearish).
	if bb, err := indicators.BollingerBands(prices, 20, 2.0); err == nil {
		snap.BBUpper = bb.Upper
		snap.BBMiddle = bb.Middle
		snap.BBLower = bb.Lower
		snap.BBPosition = bb.Position
		score += -(bb.Position*2-1) * 0.15
		totalWeight += 0.15
	}

	// SMA 7/20 golden-cross / death-cross — weight 0.20
	if sma7, err := indicators.SMA(prices, 7); err == nil {
		snap.SMAFast = sma7
		if sma20, err := indicators.SMA(prices, 20); err == nil {
			snap.SMASlow = sma20
			if sma7 > sma20 {
				score += 1.0 * 0.20
			} else {
				score += -1.0 * 0.20
			}
			totalWeight += 0.20
		}
	}

	// Stochastic %K/%D — weight 0.15
	if stoch, err := indicators.Stochastic(prices, 14, 3); err == nil {
		snap.StochK = stoch.K
		snap.StochD = stoch.D
		score += stochasticScore(stoch.K, stoch.D) * 0.15
		totalWeight += 0.15
	}

	// ATR — informational only (volatility sizing), not scored.
	if atr, err := indicators.ATR(prices, 14); err == nil {
		snap.ATR = atr
	}

	if totalWeight == 0 {
		return snap, 0, fmt.Errorf("no indicators could be computed from %d prices", len(prices))
	}
	return snap, score / totalWeight, nil
}

func rsiScore(rsi float64) float64 {
	switch {
	case rsi <= 20:
		return 1.0
	case rsi <= 30:
		return 0.7
	case rsi <= 40:
		return 0.3
	case rsi >= 80:
		return -1.0
	case rsi >= 70:
		return -0.7
	case rsi >= 60:
		return -0.3
	default:
		return 0
	}
}

func macdScore(line, hist float64) float64 {
	switch {
	case hist > 0 && line > 0:
		return 1.0 // momentum building, above zero
	case hist > 0:
		return 0.5 // momentum building, below zero
	case hist < 0 && line < 0:
		return -1.0 // momentum collapsing, below zero
	default:
		return -0.5
	}
}

func stochasticScore(k, d float64) float64 {
	switch {
	case k < 20 && d < 20:
		return 1.0 // both lines oversold
	case k < 20:
		return 0.5
	case k > 80 && d > 80:
		return -1.0 // both lines overbought
	case k > 80:
		return -0.5
	default:
		return 0
	}
}

func scoreToSignal(score float64) Signal {
	switch {
	case score >= 0.6:
		return StrongBuy
	case score >= 0.2:
		return Buy
	case score <= -0.6:
		return StrongSell
	case score <= -0.2:
		return Sell
	default:
		return Neutral
	}
}
