package backtest

import (
	"fmt"
	"testing"

	"github.com/wyplerszymon0-lab/cryptopulse/internal/predictor"
)

// The CLI fetches at most 365 days; longer series show how the cost grows.
var benchDays = []int{90, 365, 1000, 3000}

// BenchmarkScores measures the lookahead-free score series: every day
// recomputes all indicators from prices[0..i], so the cost is O(n²).
//
//	go test ./internal/backtest -run '^$' -bench Scores -benchmem
func BenchmarkScores(b *testing.B) {
	for _, n := range benchDays {
		eng := predictor.NewEngine(wavePrices(n))
		b.Run(fmt.Sprintf("days=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Scores(eng)
			}
		})
	}
}

// BenchmarkWalkForward puts Scores in context: the full --optimize run for
// one coin (scores once, then the default grid on every training window).
func BenchmarkWalkForward(b *testing.B) {
	for _, n := range benchDays[1:] {
		eng := predictor.NewEngine(wavePrices(n))
		b.Run(fmt.Sprintf("days=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := WalkForward(eng, DefaultWalkForward()); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
