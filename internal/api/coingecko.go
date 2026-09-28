package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseURL = "https://api.coingecko.com/api/v3"
	maxRetries     = 3
	maxRetryAfter  = 60 * time.Second
)

// CoinGeckoClient is a rate-limit-aware HTTP client for the CoinGecko public API.
type CoinGeckoClient struct {
	http    *http.Client
	baseURL string
	backoff time.Duration // wait before the first retry; doubles on each further retry
}

// NewCoinGeckoClient creates a client with a 20-second per-request timeout.
func NewCoinGeckoClient() *CoinGeckoClient {
	return &CoinGeckoClient{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: defaultBaseURL,
		backoff: time.Second,
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

// retryableError marks failures worth another attempt: network errors,
// rate limiting and server errors. Anything else (a 404 for an unknown coin,
// a malformed body) fails immediately instead of burning retries.
type retryableError struct {
	err        error
	retryAfter time.Duration // server-requested wait, 0 if none
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// FetchPrices retrieves daily closing prices for coinID in USD.
func (c *CoinGeckoClient) FetchPrices(ctx context.Context, coinID string, days int) ([]float64, error) {
	s, err := c.FetchSeries(ctx, coinID, days)
	return s.Prices, err
}

// FetchSeries retrieves daily closing prices and their timestamps for coinID in USD.
// Transient failures are retried up to 3 attempts in total, waiting 1s then 2s,
// or longer if the server sends Retry-After.
func (c *CoinGeckoClient) FetchSeries(ctx context.Context, coinID string, days int) (Series, error) {
	endpoint := fmt.Sprintf(
		"%s/coins/%s/market_chart?vs_currency=usd&days=%d&interval=daily",
		c.baseURL, url.PathEscape(coinID), days,
	)

	var lastErr error
	wait := c.backoff
	for attempt := 1; attempt <= maxRetries; attempt++ {
		series, err := c.fetchOnce(ctx, endpoint)
		if err == nil {
			slog.Debug("fetched prices", "coin", coinID, "days", days, "points", len(series.Prices))
			return series, nil
		}
		lastErr = err
		slog.Debug("fetch failed", "coin", coinID, "attempt", attempt, "err", err)

		var retryable *retryableError
		if !errors.As(err, &retryable) {
			return Series{}, err // the caller already names the coin
		}
		if attempt == maxRetries {
			break
		}
		delay := max(wait, retryable.retryAfter)
		slog.Debug("retry backoff", "coin", coinID, "attempt", attempt, "wait", delay)
		select {
		case <-ctx.Done():
			return Series{}, ctx.Err()
		case <-time.After(delay):
		}
		wait *= 2
	}
	return Series{}, fmt.Errorf("all %d attempts failed: %w", maxRetries, lastErr)
}

func (c *CoinGeckoClient) fetchOnce(ctx context.Context, endpoint string) (Series, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Series{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Series{}, ctx.Err() // cancelled by the caller: don't retry
		}
		return Series{}, &retryableError{err: fmt.Errorf("http: %w", err)}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests:
		return Series{}, &retryableError{
			err:        fmt.Errorf("rate limited (429) — reduce --workers or wait before retrying"),
			retryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
		}
	case resp.StatusCode >= 500:
		return Series{}, &retryableError{err: fmt.Errorf("server error HTTP %d", resp.StatusCode)}
	case resp.StatusCode == http.StatusNotFound:
		return Series{}, fmt.Errorf("unknown coin ID (HTTP 404) — use CoinGecko IDs such as bitcoin, ethereum, solana")
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

// parseRetryAfter reads a Retry-After header given in seconds, capped at one minute.
func parseRetryAfter(header string) time.Duration {
	secs, err := strconv.Atoi(header)
	if err != nil || secs <= 0 {
		return 0
	}
	return min(time.Duration(secs)*time.Second, maxRetryAfter)
}
