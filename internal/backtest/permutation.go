package backtest

import (
	"fmt"
	"math/rand/v2"
	"slices"
)

// DefaultTimingRuns is how many random placements RandomTiming draws in the CLI.
// The p-value's resolution is 1/(runs+1); 10,000 runs of a year take a few ms.
const DefaultTimingRuns = 10_000

// TimingSeed fixes the random placements, so the same prices give the same p-value.
const TimingSeed = 1

// TimingTest compares a strategy's total return with the same trades placed at
// random. Returns are in percent.
type TimingTest struct {
	Runs     int     `json:"runs"`
	Trades   int     `json:"trades"`
	DaysHeld int     `json:"days_held"`
	Actual   float64 `json:"actual_return_pct"`
	Median   float64 `json:"random_median_pct"`
	Q05      float64 `json:"random_q05_pct"`
	Q95      float64 `json:"random_q95_pct"`
	PValue   float64 `json:"p_value"` // share of random placements at least as good, one-sided
}

// RandomTiming is a permutation test of a long-only strategy's timing. It keeps
// the strategy's round trips (how many, how long each is held, the commission on
// both sides) and places them at random, without overlap, between day `from`
// (earliest entry fill) and day `to` (latest exit fill, inclusive), as many times
// as `runs`. Exposure and trade count then match the strategy's, so the only
// thing that differs is when it is in the market. As in simulate, a new entry can
// fill no earlier than the day after the previous exit.
//
// Total return is compounded from the trades, which is exactly the all-in
// strategy's return. The p-value is (1 + #placements ≥ strategy) / (runs + 1),
// so it is never 0. With a fixed seed the result is reproducible.
func RandomTiming(prices []float64, trades []Trade, from, to, runs int, seed uint64) (TimingTest, error) {
	if runs < 1 {
		return TimingTest{}, fmt.Errorf("runs must be positive, got %d", runs)
	}
	if from < 0 || to >= len(prices) || from >= to {
		return TimingTest{}, fmt.Errorf("window [%d, %d] is outside %d prices", from, to, len(prices))
	}
	durations := make([]int, len(trades))
	held := 0
	for i, t := range trades {
		durations[i] = t.ExitIndex - t.EntryIndex
		// 0 days happens: an entry filled on the window's last day is closed the same day.
		if durations[i] < 0 || t.EntryIndex < from || t.ExitIndex > to {
			return TimingTest{}, fmt.Errorf("trade %d (%d→%d) is not inside the window [%d, %d]", i, t.EntryIndex, t.ExitIndex, from, to)
		}
		held += durations[i]
	}
	k := len(trades)
	slack := to - from - held - max(k-1, 0) // free days to share among the k+1 gaps
	if slack < 0 {
		return TimingTest{}, fmt.Errorf("%d trades holding %d days do not fit in [%d, %d]", k, held, from, to)
	}

	factor := func(entry, exit int) float64 {
		return prices[exit] / prices[entry] * (1 - CommissionRate) * (1 - CommissionRate)
	}
	actual := 1.0
	for _, t := range trades {
		actual *= factor(t.EntryIndex, t.ExitIndex)
	}

	rng := rand.New(rand.NewPCG(seed, 0x5eed))
	random := make([]float64, runs)
	better := 0
	for r := range runs {
		total := 1.0
		for _, span := range place(rng, durations, from, slack) {
			total *= factor(span[0], span[1])
		}
		random[r] = total
		if total >= actual {
			better++
		}
	}

	slices.Sort(random)
	pct := func(q float64) float64 { return (random[int(q*float64(runs-1))] - 1) * 100 }
	return TimingTest{
		Runs: runs, Trades: k, DaysHeld: held,
		Actual: (actual - 1) * 100,
		Median: pct(0.5), Q05: pct(0.05), Q95: pct(0.95),
		PValue: float64(1+better) / float64(runs+1),
	}, nil
}

// place puts the holding periods in random order at random positions: a uniformly
// random composition of `slack` free days into len(durations)+1 gaps (stars and
// bars: k distinct cut points among slack+k, sorted), with one extra flat day
// after every exit. Returns [entry, exit] day pairs.
func place(rng *rand.Rand, durations []int, from, slack int) [][2]int {
	k := len(durations)
	cuts := rng.Perm(slack + k)[:k]
	slices.Sort(cuts)
	spans := make([][2]int, k)
	day, prev := from, -1
	for i, p := range rng.Perm(k) {
		day += cuts[i] - prev - 1 // the gap before this trade
		prev = cuts[i]
		spans[i] = [2]int{day, day + durations[p]}
		day += durations[p] + 1
	}
	return spans
}
