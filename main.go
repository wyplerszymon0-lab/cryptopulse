package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/api"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/backtest"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
	"github.com/wyplerszymon0-lab/cryptopulse/internal/report"
)

const version = "2.0.0"

func main() {
	coinsStr := flag.String("coins", "bitcoin,ethereum,solana", "comma-separated CoinGecko coin IDs")
	days := flag.Int("days", 90, "historical days to fetch (30–365)")
	asJSON := flag.Bool("json", false, "output as JSON (pipe-friendly)")
	verbose := flag.Bool("verbose", false, "enable debug logging")
	workers := flag.Int("workers", 3, "concurrent fetch workers")
	doBacktest := flag.Bool("backtest", false, "run walk-forward backtest instead of live analysis")
	versionFlag := flag.Bool("version", false, "print version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `CryptoPulse v%s — Real-Time Technical Analysis & Backtesting Engine

Usage:
  predictor [flags]

Flags:
`, version)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  predictor                                    Analyse BTC, ETH, SOL (90 days)
  predictor --coins bitcoin,ethereum           Custom coin list
  predictor --backtest --days 365              1-year walk-forward backtest
  predictor --json | jq '.[] | .signal'        JSON output for scripting
  predictor --coins cardano --days 60          60-day analysis

Any CoinGecko coin ID is valid: bitcoin, ethereum, solana, cardano, dogecoin, …
`)
	}

	flag.Parse()

	if *versionFlag {
		fmt.Printf("CryptoPulse v%s\n", version)
		os.Exit(0)
	}

	level := slog.LevelWarn
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if *days < 30 || *days > 365 {
		fmt.Fprintf(os.Stderr, "error: --days must be between 30 and 365\n")
		os.Exit(1)
	}

	coins := parseCoins(*coinsStr)
	if len(coins) == 0 {
		fmt.Fprintf(os.Stderr, "error: --coins must contain at least one ID\n")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := api.NewCoinGeckoClient()

	if *doBacktest {
		runBacktest(ctx, client, coins, *days, *asJSON)
	} else {
		runAnalysis(ctx, client, coins, *days, *workers, *asJSON)
	}
}

// coinResult carries the outcome of a single fetch.
type coinResult struct {
	coinID string
	prices []float64
	err    error
}

// fetchAll fetches price series for all coins concurrently, bounded by workers.
func fetchAll(ctx context.Context, client *api.CoinGeckoClient, coins []string, days, workers int) []coinResult {
	results := make([]coinResult, len(coins))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, coin := range coins {
		wg.Add(1)
		i, coin := i, coin
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			slog.Debug("fetching", "coin", coin, "days", days)
			prices, err := client.FetchPrices(ctx, coin, days)
			results[i] = coinResult{coinID: coin, prices: prices, err: err}
		}()
	}

	wg.Wait()
	return results
}

func runAnalysis(ctx context.Context, client *api.CoinGeckoClient, coins []string, days, workers int, asJSON bool) {
	if !asJSON {
		report.PrintHeader(version)
	}

	start := time.Now()
	datasets := fetchAll(ctx, client, coins, days, workers)
	elapsed := time.Since(start)

	if !asJSON {
		fmt.Printf("Fetched %d coin(s) in %s\n", len(coins), elapsed.Round(time.Millisecond))
	}

	var results []predictor.Result
	for _, d := range datasets {
		if d.err != nil {
			slog.Error("fetch failed", "coin", d.coinID, "err", d.err)
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", d.coinID, d.err)
			continue
		}
		engine := predictor.NewEngine(d.prices)
		result, err := engine.Predict()
		if err != nil {
			slog.Error("prediction failed", "coin", d.coinID, "err", err)
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", d.coinID, err)
			continue
		}
		result.CoinID = d.coinID
		results = append(results, result)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(1)
		}
		return
	}

	for _, r := range results {
		report.PrintResult(r)
	}
}

func runBacktest(ctx context.Context, client *api.CoinGeckoClient, coins []string, days int, asJSON bool) {
	if !asJSON {
		report.PrintHeader(version)
		fmt.Printf("Running walk-forward backtest: %d coin(s) × %d days…\n", len(coins), days)
	}

	datasets := fetchAll(ctx, client, coins, days, len(coins))

	var btResults []backtest.Result
	for _, d := range datasets {
		if d.err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", d.coinID, d.err)
			continue
		}
		engine := predictor.NewEngine(d.prices)
		bt, err := backtest.Run(engine)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: backtest failed for %s: %v\n", d.coinID, err)
			continue
		}
		bt.CoinID = d.coinID
		btResults = append(btResults, bt)
	}

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(btResults); err != nil {
			fmt.Fprintf(os.Stderr, "json encode: %v\n", err)
			os.Exit(1)
		}
		return
	}

	report.PrintBacktestSummary(btResults)
}

func parseCoins(s string) []string {
	var coins []string
	for _, c := range strings.Split(s, ",") {
		c = strings.TrimSpace(strings.ToLower(c))
		if c != "" {
			coins = append(coins, c)
		}
	}
	return coins
}
