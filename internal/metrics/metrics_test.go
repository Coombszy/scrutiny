package metrics_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/metrics"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newCollector(t *testing.T) (*metrics.Collector, *metrics.Memory, *clock) {
	t.Helper()
	store := metrics.NewMemory()
	c := &clock{now: time.Date(2026, 11, 11, 10, 0, 30, 0, time.UTC)}
	return metrics.NewCollector(store, c.Now), store, c
}

func TestCountsAreBucketedByMinuteAndFlushed(t *testing.T) {
	ctx := context.Background()
	col, store, clk := newCollector(t)
	col.Inc("a")
	col.Inc("a")
	clk.now = clk.now.Add(time.Minute)
	col.Add("a", 3)
	col.Inc("b")

	// Reads include counts not yet flushed.
	got, err := col.Since(ctx, clk.now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"a@10:00": 2, "a@10:01": 3, "b@10:01": 1}
	if m := byKey(got); len(m) != 3 || m["a@10:00"] != 2 || m["a@10:01"] != 3 || m["b@10:01"] != 1 {
		t.Errorf("Since = %v, want %v", m, want)
	}

	if err := col.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	col.Inc("a") // after the flush, added to the stored count
	stored, _ := store.CountsSince(ctx, clk.now.Add(-time.Hour))
	if m := byKey(stored); m["a@10:01"] != 3 {
		t.Errorf("stored = %v", m)
	}
	got, _ = col.Since(ctx, clk.now.Add(-time.Hour))
	if m := byKey(got); m["a@10:01"] != 4 {
		t.Errorf("after flush, Since = %v, want a@10:01 = 4", m)
	}
}

func TestSummaryZeroFillsTheLastHour(t *testing.T) {
	ctx := context.Background()
	col, _, clk := newCollector(t)
	col.Inc(metrics.GitHubRequests)
	clk.now = clk.now.Add(-30 * time.Minute)
	col.Add(metrics.GitHubRequests, 4)
	clk.now = clk.now.Add(-5 * time.Hour)
	col.Add(metrics.GitHubRequests, 10)                 // in the last day, not the last hour
	clk.now = clk.now.Add(30*time.Minute + 5*time.Hour) // back to 10:00:30

	sum, err := col.Summary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var req metrics.Series
	for _, s := range sum {
		if s.Name == metrics.GitHubRequests {
			req = s
		}
	}
	if req.LastHour != 5 || req.LastDay != 15 || len(req.Minutes) != 60 {
		t.Fatalf("summary = hour %d, day %d, %d minutes", req.LastHour, req.LastDay, len(req.Minutes))
	}
	if req.Minutes[59] != 1 || req.Minutes[29] != 4 || req.Max() != 4 {
		t.Errorf("minutes = %v", req.Minutes)
	}
	if len(sum) != len(metrics.Known) {
		t.Errorf("summary lists %d metrics, want every known one (%d)", len(sum), len(metrics.Known))
	}
}

func TestTransportCountsGitHubTraffic(t *testing.T) {
	ctx := context.Background()
	col, _, clk := newCollector(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graphql":
			w.Write([]byte(`{}`))
		case "/cached":
			w.WriteHeader(http.StatusNotModified)
		case "/limited":
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
		case "/broken":
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	client := &http.Client{Transport: metrics.Transport(http.DefaultTransport, col)}
	for _, p := range []string{"/repos/x", "/graphql", "/cached", "/limited", "/broken"} {
		resp, err := client.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	failing := &http.Client{Transport: metrics.Transport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no route to host")
	}), col)}
	failing.Get("http://unreachable/")

	got, _ := col.Since(ctx, clk.now.Add(-time.Minute))
	totals := map[string]int{}
	for _, b := range got {
		totals[b.Name] += b.Count
	}
	want := map[string]int{
		metrics.GitHubRequests: 6, metrics.GitHubGraphQL: 1, metrics.GitHubNotModified: 1,
		metrics.GitHubRateLimited: 1, metrics.GitHubErrors: 3, // 403, 502 and the transport error
	}
	for name, n := range want {
		if totals[name] != n {
			t.Errorf("%s = %d, want %d (all: %v)", name, totals[name], n, totals)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func byKey(bs []metrics.Bucket) map[string]int {
	m := map[string]int{}
	for _, b := range bs {
		m[b.Name+"@"+b.Minute.UTC().Format("15:04")] += b.Count
	}
	return m
}

func TestFailedGitHubRequestsAreLogged(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	col, _, _ := newCollector(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fine" {
			w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("X-GitHub-Request-Id", "ABCD:1234")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	client := &http.Client{Transport: metrics.Transport(http.DefaultTransport, col)}
	for _, p := range []string{"/fine", "/repos/o/r/pulls/1?x=1"} {
		resp, _ := client.Get(srv.URL + p)
		resp.Body.Close()
	}
	failing := &http.Client{Transport: metrics.Transport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("no route to host")
	}), col)}
	failing.Get("http://unreachable/graphql")

	out := logs.String()
	for _, want := range []string{
		`msg="GitHub request failed"`, "method=GET", `path="/repos/o/r/pulls/1?x=1"`, "status=403",
		"request_id=ABCD:1234", "rate_limit_remaining=0", "duration=",
		`path=/graphql`, `err="no route to host"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs lack %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/fine") {
		t.Errorf("a successful request was logged:\n%s", out)
	}
}

func TestCancelledRequestsAreNotGitHubFailures(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	col, _, clk := newCollector(t)
	client := &http.Client{Transport: metrics.Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	}), col)}
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://api.github.test/search/issues", nil)
	go cancel()
	client.Do(req)

	got, _ := col.Since(context.Background(), clk.now.Add(-time.Minute))
	for _, b := range got {
		if b.Name == metrics.GitHubErrors {
			t.Errorf("a cancelled request counted as a failure: %+v", got)
		}
	}
	if strings.Contains(logs.String(), "GitHub request failed") {
		t.Errorf("a cancelled request was logged as failed:\n%s", logs.String())
	}
}
