package report_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/backtest"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/report"
)

var update = flag.Bool("update", false, "update golden test files")

func checkGolden(t *testing.T, goldenFile string, got []byte) {
	t.Helper()
	goldenPath := filepath.Join("testdata", goldenFile)
	if *update {
		if err := os.MkdirAll("testdata", 0755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0644); err != nil {
			t.Fatalf("failed to write golden file %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s: %v (run with -update to generate)", goldenPath, err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("output does not match golden file %s\n--- Got ---\n%s\n--- Want ---\n%s", goldenPath, string(got), string(want))
	}
}

func sampleBacktestResults() []backtest.Result {
	return []backtest.Result{
		{
			CoinID:           "btc",
			TotalReturn:      45.2,
			AnnualizedReturn: 62.4,
			BuyHoldReturn:    20.0,
			SharpeRatio:      1.85,
			SortinoRatio:     2.40,
			MaxDrawdown:      14.2,
			WinRate:          65.5,
			TotalTrades:      24,
		},
		{
			CoinID:           "eth",
			TotalReturn:      -12.5,
			AnnualizedReturn: -15.1,
			BuyHoldReturn:    5.0,
			SharpeRatio:      0.42,
			SortinoRatio:     0.55,
			MaxDrawdown:      28.6,
			WinRate:          42.0,
			TotalTrades:      18,
		},
	}
}

func sampleWalkForwardResults() []backtest.WalkForwardResult {
	equity := make([]float64, 120)
	return []backtest.WalkForwardResult{
		{
			CoinID:   "btc",
			OOSStart: 180,
			Folds: []backtest.Fold{
				{
					TrainStart:  30,
					TestStart:   150,
					TestEnd:     180,
					Chosen:      backtest.Params{Entry: 0.3, Exit: -0.1},
					TrainSharpe: 1.72,
					TestReturn:  8.4,
				},
				{
					TrainStart:  60,
					TestStart:   180,
					TestEnd:     210,
					Chosen:      backtest.Params{Entry: 0.4, Exit: -0.2},
					TrainSharpe: 1.45,
					TestReturn:  -3.2,
				},
			},
			Optimized: backtest.Result{
				TotalReturn:  28.5,
				SharpeRatio:  1.65,
				SortinoRatio: 2.10,
				MaxDrawdown:  11.8,
				TotalTrades:  14,
				Equity:       equity,
			},
			Fixed: backtest.Result{
				TotalReturn:  15.2,
				SharpeRatio:  1.10,
				SortinoRatio: 1.35,
				MaxDrawdown:  19.4,
				TotalTrades:  20,
			},
			BuyHold: backtest.Result{
				TotalReturn:  -5.0,
				SharpeRatio:  0.25,
				SortinoRatio: 0.30,
				MaxDrawdown:  25.0,
				TotalTrades:  1,
			},
		},
	}
}

func TestPrintBacktestSummary_Empty(t *testing.T) {
	var buf bytes.Buffer
	report.PrintBacktestSummary(&buf, nil)
	expected := "No backtest results.\n"
	if got := buf.String(); got != expected {
		t.Errorf("got %q, want %q", got, expected)
	}
}

func TestPrintBacktestSummary_Golden(t *testing.T) {
	var buf bytes.Buffer
	report.PrintBacktestSummary(&buf, sampleBacktestResults())
	checkGolden(t, "backtest_summary.golden", buf.Bytes())
}

func TestPrintWalkForward_Golden(t *testing.T) {
	var buf bytes.Buffer
	report.PrintWalkForward(&buf, sampleWalkForwardResults())
	checkGolden(t, "walk_forward.golden", buf.Bytes())
}
