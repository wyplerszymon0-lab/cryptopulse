package report

import (
	"fmt"
	"io"
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

// PrintHeader writes the application banner to w.
func PrintHeader(w io.Writer, version string) {
	title := fmt.Sprintf("CryptoPulse v%s  ·  Technical Analysis & Backtesting Engine", version)
	sub := "Go  ·  Zero Dependencies  ·  7 Indicators  ·  Walk-Forward Backtest"
	// Pad by rune count: "·" is one column but two bytes.
	titleW, subW := utf8.RuneCountInString(title), utf8.RuneCountInString(sub)
	width := max(titleW, subW) + 4

	fmt.Fprintf(w, "%s╔%s╗%s\n", cyan, strings.Repeat("═", width), reset)
	fmt.Fprintf(w, "%s║%s  %s%s  %s║%s\n", cyan, bold+white, title, strings.Repeat(" ", width-4-titleW), reset+cyan, reset)
	fmt.Fprintf(w, "%s║%s  %s%s  %s║%s\n", cyan, dim+white, sub, strings.Repeat(" ", width-4-subW), reset+cyan, reset)
	fmt.Fprintf(w, "%s╚%s╝%s\n\n", cyan, strings.Repeat("═", width), reset)
}

// PrintResult writes a formatted analysis result for one coin to w.
func PrintResult(w io.Writer, r predictor.Result) {
	label := strings.ToUpper(r.CoinID)
	pad := (54 - len(label) - 2) / 2
	line := strings.Repeat("━", pad)
	fmt.Fprintf(w, "\n%s%s %s%s%s %s%s\n\n", bold+cyan, line, reset+bold+white, label, reset+bold+cyan, line, reset)

	// Price and forecast
	fColor, fSign := green, "+"
	if r.ForecastDelta < 0 {
		fColor, fSign = red, ""
	}
	fmt.Fprintf(w, "  %-22s  %s%s%s\n", "Current Price", white, indicators.FmtMoney(r.CurrentPrice), reset)
	fmt.Fprintf(w, "  %-22s  %s%s%s   %s%s%.2f%%%s   R² %.2f\n\n",
		"Forecast (1-day)", white, indicators.FmtMoney(r.Forecast), reset,
		fColor, fSign, r.ForecastDelta, reset, r.ForecastR2)

	// Signal banner
	sc := signalColor(r.Signal)
	fmt.Fprintf(w, "  Signal  %s►  %-13s%s  Score %s%+.3f%s\n\n",
		bold+sc, r.Signal.String(), reset,
		scoreColor(r.Score), r.Score, reset)

	// Indicators section
	fmt.Fprintf(w, "  %s─ Indicators ──────────────────────────────────%s\n", cyan, reset)
	ind := r.Indicators

	rsiCol, rsiLbl := rsiStatus(ind.RSI)
	indicatorRow(w, "RSI (14)", fmt.Sprintf("%.1f", ind.RSI), rsiCol+rsiLbl+reset)

	macdDir := green + "▲ Bullish"
	if ind.MACDHist < 0 {
		macdDir = red + "▼ Bearish"
	}
	indicatorRow(w, "MACD Line", fmt.Sprintf("%+.2f", ind.MACDLine),
		fmt.Sprintf("%s%s (hist %+.2f)%s", macdDir, dim, ind.MACDHist, reset))

	bbLbl := yellowStr("Mid-band")
	if ind.BBPosition < 0.2 {
		bbLbl = greenStr("Near lower  (mean-reversion)")
	} else if ind.BBPosition > 0.8 {
		bbLbl = redStr("Near upper  (overextended)")
	}
	indicatorRow(w, "Bollinger Position", fmt.Sprintf("%.1f%%", ind.BBPosition*100), bbLbl)

	crossLbl := greenStr("Golden Cross  ▲")
	if ind.SMAFast < ind.SMASlow {
		crossLbl = redStr("Death Cross  ▼")
	}
	indicatorRow(w, "SMA 7 / 20", fmtShort(ind.SMAFast)+" / "+fmtShort(ind.SMASlow), crossLbl)

	if ind.ATR > 0 {
		volPct := ind.ATR / r.CurrentPrice * 100
		volLbl := yellowStr("moderate")
		if volPct > 5 {
			volLbl = redStr("high")
		} else if volPct < 2 {
			volLbl = greenStr("low")
		}
		indicatorRow(w, "ATR (14)", indicators.FmtMoney(ind.ATR),
			fmt.Sprintf("%s volatility (%.1f%%/day)", volLbl, volPct))
	}

	if ind.StochK > 0 || ind.StochD > 0 {
		stLbl := yellowStr("Neutral")
		if ind.StochK < 20 && ind.StochD < 20 {
			stLbl = greenStr("Oversold")
		} else if ind.StochK > 80 && ind.StochD > 80 {
			stLbl = redStr("Overbought")
		}
		indicatorRow(w, "Stochastic (14,3)", fmt.Sprintf("K:%.1f D:%.1f", ind.StochK, ind.StochD), stLbl)
	}

	// Risk / confidence bar
	fmt.Fprintf(w, "\n  %s─ Risk Profile ────────────────────────────────%s\n", cyan, reset)
	indicatorRow(w, "Confidence", fmt.Sprintf("%.1f%%", r.Confidence), confidenceBar(r.Confidence))
	fmt.Fprintln(w)
}

// PrintBacktestSummary writes a comparison table of backtest results to w.
func PrintBacktestSummary(w io.Writer, results []backtest.Result) {
	if len(results) == 0 {
		fmt.Fprintln(w, "No backtest results.")
		return
	}

	fmt.Fprintf(w, "\n%s━━━━━━━━━━━━━━━━━━━━━━━━━━ BACKTEST RESULTS ━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n\n", bold+cyan, reset)

	// Header widths match the value formats below (sign, digits and unit included).
	hdr := fmt.Sprintf("  %-10s  %9s  %9s  %8s  %9s  %9s  %9s  %7s  %10s",
		"Coin", "Total Ret", "Ann. Ret", "Sharpe", "Sortino", "Max DD", "Win Rate", "Trades", "vs B&H")
	fmt.Fprintf(w, "%s%s%s\n", bold, hdr, reset)
	fmt.Fprintf(w, "  %s\n", strings.Repeat("─", 96))

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

		fmt.Fprintf(w, "  %-10s  %s%+8.1f%%%s  %+8.1f%%  %8.2f  %9.2f  %s%8.1f%%%s  %8.1f%%  %7d  %s%+8.1f%%p%s\n",
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

	fmt.Fprintf(w, "\n  %s%d  %s\n", dim, len(results), "coin(s) backtested")
	fmt.Fprintf(w, "  Trades fill at next-day close · 0.1%% commission per side · long-only\n")
	fmt.Fprintf(w, "  vs B&H: percentage-point difference vs buying and holding from warmup day%s\n\n", reset)
	fmt.Fprintf(w, "  %sWARNING: Past performance does not predict future results.%s\n\n", yellow, reset)
}

// ── helpers ──────────────────────────────────────────────────────────────────

// indicatorRow keeps every value right-aligned in one column, with labels
// starting at the same position. value must be plain ASCII (no colour codes),
// because the padding counts bytes.
func indicatorRow(w io.Writer, name, value, label string) {
	fmt.Fprintf(w, "  %-24s  %13s   %s\n", name, value, label)
}

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

// PrintWalkForward writes the out-of-sample comparison and the per-fold choices to w.
func PrintWalkForward(w io.Writer, results []backtest.WalkForwardResult) {
	fmt.Fprintf(w, "\n%s━━━━━━━━━━━━━━━━━━━━━ WALK-FORWARD OPTIMISATION ━━━━━━━━━━━━━━━━━━━━━%s\n", bold+cyan, reset)

	for _, wf := range results {
		days := len(wf.Optimized.Equity)
		fmt.Fprintf(w, "\n  %s%s%s  %sout-of-sample: last %d days, %d folds%s\n\n",
			bold, strings.ToUpper(wf.CoinID), reset, dim, days, len(wf.Folds), reset)
		fmt.Fprintf(w, "  %s%-26s  %10s  %8s  %9s  %8s  %7s  %8s%s\n", bold,
			"Strategy", "Total Ret", "Sharpe", "Sortino", "Max DD", "Trades", "Exposure", reset)
		fmt.Fprintf(w, "  %s\n", strings.Repeat("─", 88))
		rows := []struct {
			name string
			r    backtest.Result
		}{
			{"Walk-forward optimised", wf.Optimized},
			{"Fixed default (±0.2)", wf.Fixed},
			{"Fixed ±0.2, vol-targeted", wf.VolTargeted},
			{"Buy & hold", wf.BuyHold},
		}
		for _, row := range rows {
			col := green
			if row.r.TotalReturn < 0 {
				col = red
			}
			fmt.Fprintf(w, "  %-26s  %s%+9.1f%%%s  %8.2f  %9.2f  %s%7.1f%%%s  %7d  %7.0f%%\n",
				row.name, col, row.r.TotalReturn, reset, row.r.SharpeRatio, row.r.SortinoRatio,
				red, -row.r.MaxDrawdown, reset, row.r.TotalTrades, row.r.Exposure)
		}

		fmt.Fprintf(w, "\n  %sFold  Test days   Chosen entry/exit   Train Sharpe   Test return%s\n", dim, reset)
		for i, f := range wf.Folds {
			col := green
			if f.TestReturn < 0 {
				col = red
			}
			fmt.Fprintf(w, "  %4d  %3d–%-3d      %+.1f / %+.1f          %6.2f       %s%+7.1f%%%s\n",
				i+1, f.TestStart, f.TestEnd-1, f.Chosen.Entry, f.Chosen.Exit, f.TrainSharpe, col, f.TestReturn, reset)
		}
	}

	fmt.Fprintf(w, "\n  %sParameters are chosen by in-sample Sharpe on each training window and only\n", dim)
	fmt.Fprintf(w, "  scored on the following test window, so every number above is out-of-sample.%s\n", reset)
	fmt.Fprintf(w, "  %sWARNING: Past performance does not predict future results.%s\n\n", yellow, reset)
}
