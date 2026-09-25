package report

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/backtest"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/indicators"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	green  = "\033[32m"
	red    = "\033[31m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	white  = "\033[97m"
)

// PrintHeader prints the application banner.
func PrintHeader(version string) {
	title := fmt.Sprintf("CryptoPulse v%s  ·  Technical Analysis & Backtesting Engine", version)
	sub := "Go  ·  Zero Dependencies  ·  7 Indicators  ·  Walk-Forward Backtest"
	// Pad by rune count: "·" is one column but two bytes.
	titleW, subW := utf8.RuneCountInString(title), utf8.RuneCountInString(sub)
	width := max(titleW, subW) + 4

	fmt.Printf("%s╔%s╗%s\n", cyan, strings.Repeat("═", width), reset)
	fmt.Printf("%s║%s  %s%s  %s║%s\n", cyan, bold+white, title, strings.Repeat(" ", width-4-titleW), reset+cyan, reset)
	fmt.Printf("%s║%s  %s%s  %s║%s\n", cyan, dim+white, sub, strings.Repeat(" ", width-4-subW), reset+cyan, reset)
	fmt.Printf("%s╚%s╝%s\n\n", cyan, strings.Repeat("═", width), reset)
}

// PrintResult prints a formatted analysis result for one coin.
func PrintResult(r predictor.Result) {
	label := strings.ToUpper(r.CoinID)
	pad := (54 - len(label) - 2) / 2
	line := strings.Repeat("━", pad)
	fmt.Printf("\n%s%s %s%s%s %s%s\n\n", bold+cyan, line, reset+bold+white, label, reset+bold+cyan, line, reset)

	// Price and forecast
	fColor, fSign := green, "+"
	if r.ForecastDelta < 0 {
		fColor, fSign = red, ""
	}
	fmt.Printf("  %-22s  %s%s%s\n", "Current Price", white, indicators.FmtMoney(r.CurrentPrice), reset)
	fmt.Printf("  %-22s  %s%s%s   %s%s%.2f%%%s   R² %.2f\n\n",
		"Forecast (1-day)", white, indicators.FmtMoney(r.Forecast), reset,
		fColor, fSign, r.ForecastDelta, reset, r.ForecastR2)

	// Signal banner
	sc := signalColor(r.Signal)
	fmt.Printf("  Signal  %s►  %-13s%s  Score %s%+.3f%s\n\n",
		bold+sc, r.Signal.String(), reset,
		scoreColor(r.Score), r.Score, reset)

	// Indicators section
	fmt.Printf("  %s─ Indicators ──────────────────────────────────%s\n", cyan, reset)
	ind := r.Indicators

	rsiCol, rsiLbl := rsiStatus(ind.RSI)
	fmt.Printf("  %-24s  %6.1f   %s%s%s\n", "RSI (14)", ind.RSI, rsiCol, rsiLbl, reset)

	macdDir := green + "▲ Bullish"
	if ind.MACDHist < 0 {
		macdDir = red + "▼ Bearish"
	}
	fmt.Printf("  %-24s  %+7.2f   %s%s (hist %+.2f)%s\n",
		"MACD Line", ind.MACDLine, macdDir, dim, ind.MACDHist, reset)

	bbLbl := yellowStr("Mid-band")
	if ind.BBPosition < 0.2 {
		bbLbl = greenStr("Near lower  (mean-reversion)")
	} else if ind.BBPosition > 0.8 {
		bbLbl = redStr("Near upper  (overextended)")
	}
	fmt.Printf("  %-24s  %5.1f%%   %s\n", "Bollinger Position", ind.BBPosition*100, bbLbl)

	crossLbl := greenStr("Golden Cross  ▲")
	if ind.SMAFast < ind.SMASlow {
		crossLbl = redStr("Death Cross  ▼")
	}
	fmt.Printf("  %-24s  %-9s  %s\n", "SMA 7 / 20",
		fmt.Sprintf("%s / %s", fmtShort(ind.SMAFast), fmtShort(ind.SMASlow)), crossLbl)

	if ind.ATR > 0 {
		volPct := ind.ATR / r.CurrentPrice * 100
		volLbl := yellowStr("moderate")
		if volPct > 5 {
			volLbl = redStr("high")
		} else if volPct < 2 {
			volLbl = greenStr("low")
		}
		fmt.Printf("  %-24s  %-9s  %s volatility (%.1f%%/day)\n",
			"ATR (14)", indicators.FmtMoney(ind.ATR), volLbl, volPct)
	}

	if ind.StochK > 0 || ind.StochD > 0 {
		stLbl := yellowStr("Neutral")
		if ind.StochK < 20 && ind.StochD < 20 {
			stLbl = greenStr("Oversold")
		} else if ind.StochK > 80 && ind.StochD > 80 {
			stLbl = redStr("Overbought")
		}
		fmt.Printf("  %-24s  K:%-5.1f D:%-5.1f  %s\n", "Stochastic (14,3)", ind.StochK, ind.StochD, stLbl)
	}

	// Risk / confidence bar
	fmt.Printf("\n  %s─ Risk Profile ────────────────────────────────%s\n", cyan, reset)
	fmt.Printf("  %-24s  %.1f%%   %s\n\n", "Confidence", r.Confidence, confidenceBar(r.Confidence))
}

// PrintBacktestSummary prints a formatted comparison table of backtest results.
func PrintBacktestSummary(results []backtest.Result) {
	if len(results) == 0 {
		fmt.Println("No backtest results.")
		return
	}

	fmt.Printf("\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━ BACKTEST RESULTS ━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", bold+cyan, reset)

	hdr := fmt.Sprintf("  %-10s  %-10s  %-10s  %-8s  %-9s  %-9s  %-9s  %-7s  %-10s",
		"Coin", "Total Ret", "Ann. Ret", "Sharpe", "Sortino", "Max DD", "Win Rate", "Trades", "vs B&H")
	fmt.Printf("%s%s%s\n", bold, hdr, reset)
	fmt.Printf("  %s\n", strings.Repeat("─", 94))

	for _, r := range results {
		vsB := r.TotalReturn - r.BuyHoldReturn
		retCol := green
		if r.TotalReturn < 0 {
			retCol = red
		}
		vsBCol := green
		if vsB < 0 {
			vsBCol = red
		}

		fmt.Printf("  %-10s  %s%+8.1f%%%s  %+8.1f%%  %8.2f  %9.2f  %s%8.1f%%%s  %8.1f%%  %7d  %s%+8.1f%%p%s\n",
			strings.ToUpper(r.CoinID),
			retCol, r.TotalReturn, reset,
			r.AnnualizedReturn,
			r.SharpeRatio,
			r.SortinoRatio,
			red, -r.MaxDrawdown, reset,
			r.WinRate,
			r.TotalTrades,
			vsBCol, vsB, reset,
		)
	}

	fmt.Printf("\n  %s%d  %s\n", dim, len(results), "coin(s) backtested")
	fmt.Printf("  Trades fill at next-day close · 0.1%% commission per side · long-only\n")
	fmt.Printf("  vs B&H: percentage-point difference vs buying and holding from warmup day%s\n\n", reset)
	fmt.Printf("  %sWARNING: Past performance does not predict future results.%s\n\n", yellow, reset)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func signalColor(s predictor.Signal) string {
	if s >= predictor.Buy {
		return green
	}
	if s <= predictor.Sell {
		return red
	}
	return yellow
}

func scoreColor(score float64) string {
	if score >= 0.2 {
		return green
	}
	if score <= -0.2 {
		return red
	}
	return yellow
}

func rsiStatus(rsi float64) (color, label string) {
	switch {
	case rsi <= 30:
		return green, "Oversold"
	case rsi >= 70:
		return red, "Overbought"
	default:
		return yellow, "Neutral"
	}
}

func confidenceBar(pct float64) string {
	filled := int(math.Round(pct / 5))
	if filled > 20 {
		filled = 20
	}
	c := green
	if pct < 40 {
		c = yellow
	}
	return c + strings.Repeat("█", filled) + dim + strings.Repeat("░", 20-filled) + reset
}

func greenStr(s string) string  { return green + s + reset }
func redStr(s string) string    { return red + s + reset }
func yellowStr(s string) string { return yellow + s + reset }

func fmtShort(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.2fM", v/1_000_000)
	case v >= 1000:
		return fmt.Sprintf("%.1fk", v/1000)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// PrintWalkForward prints the out-of-sample comparison and the per-fold choices.
func PrintWalkForward(results []backtest.WalkForwardResult) {
	fmt.Printf("\n%s━━━━━━━━━━━━━━━━━━━━━ WALK-FORWARD OPTIMISATION ━━━━━━━━━━━━━━━━━━━━━%s\n", bold+cyan, reset)

	for _, wf := range results {
		days := len(wf.Optimized.Equity)
		fmt.Printf("\n  %s%s%s  %sout-of-sample: last %d days, %d folds%s\n\n",
			bold, strings.ToUpper(wf.CoinID), reset, dim, days, len(wf.Folds), reset)
		fmt.Printf("  %s%-22s  %10s  %8s  %9s  %8s  %7s%s\n", bold,
			"Strategy", "Total Ret", "Sharpe", "Sortino", "Max DD", "Trades", reset)
		fmt.Printf("  %s\n", strings.Repeat("─", 72))
		rows := []struct {
			name string
			r    backtest.Result
		}{
			{"Walk-forward optimised", wf.Optimized},
			{"Fixed default (±0.2)", wf.Fixed},
			{"Buy & hold", wf.BuyHold},
		}
		for _, row := range rows {
			col := green
			if row.r.TotalReturn < 0 {
				col = red
			}
			fmt.Printf("  %-22s  %s%+9.1f%%%s  %8.2f  %9.2f  %s%7.1f%%%s  %7d\n",
				row.name, col, row.r.TotalReturn, reset, row.r.SharpeRatio, row.r.SortinoRatio,
				red, -row.r.MaxDrawdown, reset, row.r.TotalTrades)
		}

		fmt.Printf("\n  %sFold  Test days   Chosen entry/exit   Train Sharpe   Test return%s\n", dim, reset)
		for i, f := range wf.Folds {
			col := green
			if f.TestReturn < 0 {
				col = red
			}
			fmt.Printf("  %4d  %3d–%-3d      %+.1f / %+.1f          %6.2f       %s%+7.1f%%%s\n",
				i+1, f.TestStart, f.TestEnd-1, f.Chosen.Entry, f.Chosen.Exit, f.TrainSharpe, col, f.TestReturn, reset)
		}
	}

	fmt.Printf("\n  %sParameters are chosen by in-sample Sharpe on each training window and only\n", dim)
	fmt.Printf("  scored on the following test window, so every number above is out-of-sample.%s\n", reset)
	fmt.Printf("  %sWARNING: Past performance does not predict future results.%s\n\n", yellow, reset)
}
