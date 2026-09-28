package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"prices":[[1758844800000,109000.5],[1758931200000,110250.25]]}`

// newTestClient points a client at a fake CoinGecko that answers with `respond`
// and counts requests. Backoff is shortened so retry tests run fast.
func newTestClient(t *testing.T, respond func(w http.ResponseWriter, r *http.Request, n int32)) (*CoinGeckoClient, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, r, atomic.AddInt32(&calls, 1))
	}))
	t.Cleanup(srv.Close)
	return &CoinGeckoClient{http: srv.Client(), baseURL: srv.URL, backoff: time.Millisecond}, &calls
}

func TestFetchSeries_ParsesPricesAndSendsExpectedRequest(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		if r.URL.Path != "/coins/bitcoin/market_chart" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("vs_currency") != "usd" || q.Get("days") != "90" || q.Get("interval") != "daily" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		w.Write([]byte(okBody))
	})

	s, err := client.FetchSeries(context.Background(), "bitcoin", 90)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Prices) != 2 || s.Prices[1] != 110250.25 || s.Times[0] != 1758844800000 {
		t.Errorf("series = %+v", s)
	}
	if *calls != 1 {
		t.Errorf("calls = %d, want 1", *calls)
	}
}

func TestFetchSeries_EscapesCoinID(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request, _ int32) {
		if r.URL.Path != "/coins/bitcoin?x=1/market_chart" || r.URL.Query().Get("days") != "30" {
			t.Errorf("coin ID leaked into the URL: path=%q query=%q", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(okBody))
	})
	if _, err := client.FetchSeries(context.Background(), "bitcoin?x=1", 30); err != nil {
		t.Fatal(err)
	}
}

func TestFetchSeries_RetriesTransientErrors(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusBadGateway} {
		client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, n int32) {
			if n == 1 {
				w.WriteHeader(status)
				return
			}
			w.Write([]byte(okBody))
		})
		if _, err := client.FetchSeries(context.Background(), "bitcoin", 30); err != nil {
			t.Fatalf("HTTP %d then 200: %v", status, err)
		}
		if *calls != 2 {
			t.Errorf("HTTP %d: calls = %d, want 2", status, *calls)
		}
	}
}

func TestFetchSeries_GivesUpAfterThreeAttempts(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := client.FetchSeries(context.Background(), "bitcoin", 30)
	if err == nil || !strings.Contains(err.Error(), "all 3 attempts failed") {
		t.Fatalf("err = %v", err)
	}
	if *calls != 3 {
		t.Errorf("calls = %d, want 3", *calls)
	}
}

func TestFetchSeries_DoesNotRetryPermanentErrors(t *testing.T) {
	cases := map[string]func(w http.ResponseWriter){
		"unknown coin":    func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) },
		"bad request":     func(w http.ResponseWriter) { w.WriteHeader(http.StatusBadRequest) },
		"malformed JSON":  func(w http.ResponseWriter) { w.Write([]byte(`{"prices":[`)) },
		"empty series":    func(w http.ResponseWriter) { w.Write([]byte(`{"prices":[]}`)) },
		"malformed entry": func(w http.ResponseWriter) { w.Write([]byte(`{"prices":[[1758844800000]]}`)) },
	}
	for name, respond := range cases {
		t.Run(name, func(t *testing.T) {
			client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ int32) { respond(w) })
			if _, err := client.FetchSeries(context.Background(), "bitcoin", 30); err == nil {
				t.Fatal("expected an error")
			}
			if *calls != 1 {
				t.Errorf("calls = %d, want 1 (no retries)", *calls)
			}
		})
	}
}

func TestFetchSeries_HonoursRetryAfter(t *testing.T) {
	client, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, n int32) {
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(okBody))
	})
	start := time.Now()
	if _, err := client.FetchSeries(context.Background(), "bitcoin", 30); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("retried after %v, want ≥ 1s from Retry-After", elapsed)
	}
}

func TestFetchSeries_StopsWhenContextIsCancelled(t *testing.T) {
	client, calls := newTestClient(t, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	client.backoff = time.Hour // the only way out of the wait is cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.FetchSeries(ctx, "bitcoin", 30)
	if err != context.DeadlineExceeded {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if time.Since(start) > 5*time.Second || *calls != 1 {
		t.Errorf("took %v with %d calls; cancellation should end the backoff", time.Since(start), *calls)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := map[string]time.Duration{"": 0, "abc": 0, "-3": 0, "2": 2 * time.Second, "3600": maxRetryAfter}
	for in, want := range cases {
		if got := parseRetryAfter(in); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", in, got, want)
		}
	}
}
