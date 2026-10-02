package report

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/backtest"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
)

// Regenerate the golden files after an intended change with:
//
//	go test ./internal/report -update
var update = flag.Bool("update", false, "rewrite testdata/*.golden from the current output")

var ansi = regexp.MustCompile("\033\\[[0-9;]*m")

// stripANSI removes colour codes so golden files stay readable in a diff.
// Colours are checked separately in TestColours.
func stripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	// Tolerate CRLF checkouts on Windows.
	if w := strings.ReplaceAll(string(want), "\r\n", "\n"); got != w {
		t.Errorf("output differs from %s (run with -update if the change is intended)\n--- got ---\n%s\n--- want ---\n%s", path, got, w)
	}
}

func render(f func(w *bytes.Buffer)) (raw, plain string) {
	var buf bytes.Buffer
	f(&buf)
	return buf.String(), stripANSI(buf.String())
}

// ── fixtures: fixed, hand-written results (no network, no randomness) ───────

var analysis = predictor.Result{
	CoinID:        "bitcoin",
	CurrentPrice:  63250.42,
	Forecast:      63910.10,
	ForecastDelta: 1.04,
	ForecastR2:    0.82,
	Signal:        predictor.Buy,
	Score:         0.274,
	Confidence:    61.5,
	Indicators: predictor.IndicatorSnapshot{
		RSI: 58.3, MACDLine: 412.55, MACDHist: -35.20,
		BBPosition: 0.64, SMAFast: 62980, SMASlow: 61450,
		ATR: 1580.75, StochK: 71.2, StochD: 66.9,
	},
}

var summaries = []backtest.Result{
	{CoinID: "bitcoin", TotalReturn: 18.4, AnnualizedReturn: 9.1, BuyHoldReturn: 25.0,
		SharpeRatio: 0.71, SortinoRatio: 1.02, MaxDrawdown: 21.3, WinRate: 55.6, TotalTrades: 9},
	{CoinID: "solana", TotalReturn: -7.25, AnnualizedReturn: -3.6, BuyHoldReturn: -30.5,
		SharpeRatio: -0.12, SortinoRatio: -0.18, MaxDrawdown: 34.8, WinRate: 40.0, TotalTrades: 15},
}

var walkForward = []backtest.WalkForwardResult{{
	CoinID: "ethereum",
	Folds: []backtest.Fold{
		{TestStart: 120, TestEnd: 150, Chosen: backtest.Params{Entry: 0.3, Exit: -0.1}, TrainSharpe: 1.25, TestReturn: 4.2},
		{TestStart: 150, TestEnd: 180, Chosen: backtest.Params{Entry: 0.2, Exit: -0.2}, TrainSharpe: 0.87, TestReturn: -2.9},
	},
	Optimized:   backtest.Result{TotalReturn: 1.2, SharpeRatio: 0.15, SortinoRatio: 0.21, MaxDrawdown: 12.4, TotalTrades: 6, Exposure: 48, Equity: make([]float64, 60)},
	Fixed:       backtest.Result{TotalReturn: 3.4, SharpeRatio: 0.42, SortinoRatio: 0.6, MaxDrawdown: 10.1, TotalTrades: 5, Exposure: 52},
	VolTargeted: backtest.Result{TotalReturn: 2.8, SharpeRatio: 0.55, SortinoRatio: 0.79, MaxDrawdown: 6.3, TotalTrades: 5, Exposure: 31},
	BuyHold:     backtest.Result{TotalReturn: -5.6, SharpeRatio: -0.2, SortinoRatio: -0.27, MaxDrawdown: 18.9, TotalTrades: 1, Exposure: 100},
}}

// ── golden files ─────────────────────────────────────────────────────────────

func TestHeaderGolden(t *testing.T) {
	_, got := render(func(w *bytes.Buffer) { PrintHeader(w, "2.1.0") })
	checkGolden(t, "header", got)
}

func TestResultGolden(t *testing.T) {
	_, got := render(func(w *bytes.Buffer) { PrintResult(w, analysis) })
	checkGolden(t, "result", got)
}

func TestBacktestSummaryGolden(t *testing.T) {
	_, got := render(func(w *bytes.Buffer) { PrintBacktestSummary(w, summaries) })
	checkGolden(t, "backtest_summary", got)
}

func TestWalkForwardGolden(t *testing.T) {
	_, got := render(func(w *bytes.Buffer) { PrintWalkForward(w, walkForward) })
	checkGolden(t, "walk_forward", got)
}

// ── properties the golden files would only catch indirectly ─────────────────

// The banner once came out ragged because "·" is two bytes but one column.
func TestHeaderBoxIsAligned(t *testing.T) {
	for _, version := range []string{"2.1.0", "0.0.0-SNAPSHOT-5687f19"} {
		_, out := render(func(w *bytes.Buffer) { PrintHeader(w, version) })
		lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
		if len(lines) != 4 {
			t.Fatalf("version %q: want 4 banner lines, got %d", version, len(lines))
		}
		width := utf8.RuneCountInString(lines[0])
		for i, l := range lines {
			if n := utf8.RuneCountInString(l); n != width {
				t.Errorf("version %q: line %d is %d columns wide, want %d:\n%s", version, i+1, n, width, out)
			}
		}
	}
}

func TestEmptyBacktestSummary(t *testing.T) {
	_, out := render(func(w *bytes.Buffer) { PrintBacktestSummary(w, nil) })
	if out != "No backtest results.\n" {
		t.Errorf("got %q", out)
	}
}

func TestColours(t *testing.T) {
	raw, _ := render(func(w *bytes.Buffer) { PrintBacktestSummary(w, summaries) })
	if !strings.Contains(raw, green+"   +18.4%"+reset) {
		t.Error("positive total return should be green")
	}
	if !strings.Contains(raw, red+"    -7.2%"+reset) {
		t.Error("negative total return should be red")
	}

	raw, _ = render(func(w *bytes.Buffer) { PrintResult(w, analysis) })
	if !strings.Contains(raw, bold+green+"►  BUY") {
		t.Error("a BUY signal should be bold green")
	}
}
