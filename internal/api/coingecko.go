package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"
)

const (
	baseURL    = "https://api.coingecko.com/api/v3"
	maxRetries = 3
)

// CoinGeckoClient is a rate-limit-aware HTTP client for the CoinGecko public API.
type CoinGeckoClient struct {
	http *http.Client
}

// NewCoinGeckoClient creates a client with a 20-second per-request timeout.
func NewCoinGeckoClient() *CoinGeckoClient {
	return &CoinGeckoClient{
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

type marketChartResponse struct {
	Prices [][]float64 `json:"prices"`
}

// Series is a daily USD price series with its timestamps (Unix milliseconds).
type Series struct {
	Times  []int64
	Prices []float64
}

// FetchPrices retrieves daily closing prices for coinID in USD.
func (c *CoinGeckoClient) FetchPrices(ctx context.Context, coinID string, days int) ([]float64, error) {
	s, err := c.FetchSeries(ctx, coinID, days)
	return s.Prices, err
}

// FetchSeries retrieves daily closing prices and their timestamps for coinID in USD.
// On transient failures it retries up to 3 times with exponential backoff (1s, 2s, 4s).
func (c *CoinGeckoClient) FetchSeries(ctx context.Context, coinID string, days int) (Series, error) {
	url := fmt.Sprintf(
		"%s/coins/%s/market_chart?vs_currency=usd&days=%d&interval=daily",
		baseURL, coinID, days,
	)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			slog.Debug("retry backoff", "coin", coinID, "attempt", attempt, "wait", backoff)
			select {
			case <-ctx.Done():
				return Series{}, ctx.Err()
			case <-time.After(backoff):
			}
		}

		series, err := c.fetchOnce(ctx, url)
		if err == nil {
			slog.Debug("fetched prices", "coin", coinID, "days", days, "points", len(series.Prices))
			return series, nil
		}
		lastErr = err
		slog.Debug("fetch failed", "coin", coinID, "attempt", attempt+1, "err", err)
	}
	return Series{}, fmt.Errorf("%s: all %d attempts failed: %w", coinID, maxRetries, lastErr)
}

func (c *CoinGeckoClient) fetchOnce(ctx context.Context, url string) (Series, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Series{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Series{}, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return Series{}, fmt.Errorf("rate limited (429) — reduce --workers or wait before retrying")
	case http.StatusOK:
	default:
		return Series{}, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
	}

	var data marketChartResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return Series{}, fmt.Errorf("decode: %w", err)
	}
	if len(data.Prices) == 0 {
		return Series{}, fmt.Errorf("empty price series returned by API")
	}

	series := Series{Times: make([]int64, len(data.Prices)), Prices: make([]float64, len(data.Prices))}
	for i, p := range data.Prices {
		if len(p) < 2 {
			return Series{}, fmt.Errorf("malformed price entry at index %d", i)
		}
		series.Times[i] = int64(p[0])
		series.Prices[i] = p[1]
	}
	return series, nil
}
