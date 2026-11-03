package backtest

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
)

func TestPlace_StaysInTheWindowWithoutOverlap(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	durations := []int{5, 1, 12, 3}
	from, to := 31, 120
	slack := to - from - 21 - 3
	seenStart, seenEnd := false, false
	for range 5000 {
		spans := place(rng, durations, from, slack)
		held := 0
		for i, s := range spans {
			if s[0] < from || s[1] > to || s[1] <= s[0] {
				t.Fatalf("span %v outside [%d, %d]", s, from, to)
			}
			if i > 0 && s[0] <= spans[i-1][1] {
				t.Fatalf("spans %v and %v overlap or touch", spans[i-1], s)
			}
			held += s[1] - s[0]
		}
		if held != 21 {
			t.Fatalf("held %d days, want 21", held)
		}
		seenStart = seenStart || spans[0][0] == from
		seenEnd = seenEnd || spans[len(spans)-1][1] == to
	}
	if !seenStart || !seenEnd {
		t.Error("placements never reach the window's edges: the gaps are not uniform")
	}
}

func TestRandomTiming_KeepsSameDayRoundTrips(t *testing.T) {
	// Entered on the last day and closed at the window's end: only commission.
	prices := trendPrices(100, 100, 0.5)
	tt, err := RandomTiming(prices, []Trade{{EntryIndex: 40, ExitIndex: 50}, {EntryIndex: 99, ExitIndex: 99}}, 31, 99, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := (prices[50]/prices[40]*math.Pow(1-CommissionRate, 4) - 1) * 100
	if math.Abs(tt.Actual-want) > 1e-9 || tt.DaysHeld != 10 {
		t.Errorf("got %+v, want return %.6f%%", tt, want)
	}
}

func TestPlace_FillsAWindowWithNoSlackExactly(t *testing.T) {
	spans := place(rand.New(rand.NewPCG(3, 4)), []int{2, 2}, 10, 0)
	// 2 days held, 1 flat day, 2 days held: 10→12, 13→15, in either order of equal lengths.
	if spans[0] != [2]int{10, 12} || spans[1] != [2]int{13, 15} {
		t.Errorf("got %v", spans)
	}
}

func TestRandomTiming_ReproducesTheStrategyReturnFromItsTrades(t *testing.T) {
	prices := trendPrices(200, 100, 0.3)
	for i := range prices {
		prices[i] += 8 * math.Sin(float64(i)/6) // swings for the signal to trade
	}
	res, err := RunWithParams(predictor.NewEngine(prices), DefaultParams)
	if err != nil || res.TotalTrades == 0 {
		t.Fatalf("backtest: %v, %d trades", err, res.TotalTrades)
	}
	tt, err := RandomTiming(prices, res.Trades, WarmupPeriod+1, len(prices)-1, 200, 7)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(tt.Actual-res.TotalReturn) > 1e-9 {
		t.Errorf("return from trades %.12f%%, backtest %.12f%%", tt.Actual, res.TotalReturn)
	}
	if tt.Trades != res.TotalTrades || tt.Q05 > tt.Median || tt.Median > tt.Q95 {
		t.Errorf("summary out of order: %+v", tt)
	}
}

func TestRandomTiming_PerfectTimingHasATinyPValue(t *testing.T) {
	// A zig-zag: 10 days up, 10 days down. Holding every rise is the best timing there is.
	prices := make([]float64, 200)
	for i := range prices {
		if (i/10)%2 == 0 {
			prices[i] = 100 + float64(i%10)
		} else {
			prices[i] = 110 - float64(i%10)
		}
	}
	var trades []Trade
	for start := 20; start+10 < len(prices); start += 20 {
		trades = append(trades, Trade{EntryIndex: start, ExitIndex: start + 9})
	}
	tt, err := RandomTiming(prices, trades, 20, len(prices)-1, 2000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if tt.PValue > 0.001 || tt.Actual <= tt.Q95 {
		t.Errorf("perfect timing should beat every random placement: %+v", tt)
	}
}

func TestRandomTiming_LuckyTimingOnARandomWalkIsNotSignificantOnAverage(t *testing.T) {
	// On a random walk no timing has an edge, so p-values of fixed trades should
	// spread over (0, 1]: about half of them above 0.5.
	above := 0
	for seed := range uint64(40) {
		rng := rand.New(rand.NewPCG(seed, 99))
		prices := make([]float64, 300)
		prices[0] = 100
		for i := 1; i < len(prices); i++ {
			prices[i] = prices[i-1] * math.Exp(0.03*rng.NormFloat64())
		}
		trades := []Trade{{EntryIndex: 40, ExitIndex: 70}, {EntryIndex: 120, ExitIndex: 135}, {EntryIndex: 200, ExitIndex: 260}}
		tt, err := RandomTiming(prices, trades, 31, len(prices)-1, 500, seed)
		if err != nil {
			t.Fatal(err)
		}
		if tt.PValue > 0.5 {
			above++
		}
	}
	if above < 10 || above > 30 {
		t.Errorf("%d of 40 random-walk p-values above 0.5; expected about 20", above)
	}
}

func TestRandomTiming_IsReproducibleAndValidatesInput(t *testing.T) {
	prices := trendPrices(100, 100, 0.5)
	trades := []Trade{{EntryIndex: 40, ExitIndex: 50}}
	a, _ := RandomTiming(prices, trades, 31, 99, 300, 42)
	b, _ := RandomTiming(prices, trades, 31, 99, 300, 42)
	if a != b {
		t.Errorf("same seed, different results: %+v vs %+v", a, b)
	}
	none, err := RandomTiming(prices, nil, 31, 99, 100, 1)
	if err != nil || none.PValue != 1 || none.Actual != 0 {
		t.Errorf("no trades: %+v, %v", none, err)
	}
	for _, bad := range []struct {
		trades       []Trade
		from, to, nr int
	}{
		{trades, 31, 99, 0},   // no runs
		{trades, 45, 99, 10},  // trade starts before the window
		{trades, 31, 100, 10}, // window past the prices
		{[]Trade{{EntryIndex: 31, ExitIndex: 99}, {EntryIndex: 40, ExitIndex: 41}}, 31, 99, 10}, // do not fit
	} {
		if _, err := RandomTiming(prices, bad.trades, bad.from, bad.to, bad.nr, 1); err == nil {
			t.Errorf("expected an error for %+v", bad)
		}
	}
}
