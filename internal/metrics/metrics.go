// Package metrics counts what Scrutiny does, such as GitHub requests and
// their failures, per minute, keeping a week of history in a Store.
package metrics

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// The counted events.
const (
	GitHubRequests    = "github.requests"     // every GitHub API request
	GitHubGraphQL     = "github.graphql"      // those that were GraphQL
	GitHubNotModified = "github.not_modified" // answered 304: the cached copy was current
	GitHubErrors      = "github.errors"       // failed to send, or answered 4xx/5xx
	GitHubRateLimited = "github.rate_limited" // refused for the rate limit
	GitFetches        = "git.fetches"
	GitFetchErrors    = "git.fetch_errors"
)

// Metric describes a counted event for display.
type Metric struct {
	Name, Label string
	Bad         bool // counts of it are a problem
}

// Known lists the metrics in display order.
var Known = []Metric{
	{GitHubRequests, "GitHub requests", false},
	{GitHubGraphQL, "of which GraphQL", false},
	{GitHubNotModified, "Answered from cache (304)", false},
	{GitHubErrors, "GitHub failures", true},
	{GitHubRateLimited, "Rate limited", true},
	{GitFetches, "Git fetches", false},
	{GitFetchErrors, "Git fetch failures", true},
}

// Retention is how long counts are kept.
const Retention = 7 * 24 * time.Hour

// Bucket is how many times an event happened in one minute.
type Bucket struct {
	Name   string
	Minute time.Time // the start of the minute
	Count  int
}

// Store keeps Buckets; adding to an existing Bucket sums the counts.
type Store interface {
	AddCounts(ctx context.Context, buckets []Bucket) error
	CountsSince(ctx context.Context, since time.Time) ([]Bucket, error)
	PruneCounts(ctx context.Context, before time.Time) error
}

type key struct {
	name   string
	minute int64
}

// Collector counts events in memory and flushes them to a Store, so
// counting never waits on the database.
type Collector struct {
	store Store
	now   func() time.Time

	mu      sync.Mutex
	pending map[key]int
}

func NewCollector(store Store, now func() time.Time) *Collector {
	if now == nil {
		now = time.Now
	}
	return &Collector{store: store, now: now, pending: map[key]int{}}
}

// Inc counts one event.
func (c *Collector) Inc(name string) { c.Add(name, 1) }

// Add counts n events. A nil Collector counts nothing.
func (c *Collector) Add(name string, n int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending[key{name, c.now().Truncate(time.Minute).Unix()}] += n
}

func (c *Collector) takePending() []Bucket {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bucketsLocked(true)
}

func (c *Collector) bucketsLocked(clear bool) []Bucket {
	out := make([]Bucket, 0, len(c.pending))
	for k, n := range c.pending {
		out = append(out, Bucket{Name: k.name, Minute: time.Unix(k.minute, 0).UTC(), Count: n})
	}
	if clear {
		c.pending = map[key]int{}
	}
	return out
}

// Flush writes the pending counts to the Store.
func (c *Collector) Flush(ctx context.Context) error {
	pending := c.takePending()
	if len(pending) == 0 {
		return nil
	}
	if err := c.store.AddCounts(ctx, pending); err != nil {
		// Keep them for the next flush.
		for _, b := range pending {
			c.Add(b.Name, b.Count)
		}
		return err
	}
	return nil
}

// Run flushes every interval and prunes old counts, until ctx is done; it
// flushes once more on the way out.
func (c *Collector) Run(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := c.Flush(context.Background()); err != nil {
				slog.Warn("flushing metrics", "err", err)
			}
			return
		case <-tick.C:
			if err := c.Flush(ctx); err != nil {
				slog.Warn("flushing metrics", "err", err)
			}
			_ = c.store.PruneCounts(ctx, c.now().Add(-Retention))
		}
	}
}

// Since returns the counts from since on, stored and pending together.
func (c *Collector) Since(ctx context.Context, since time.Time) ([]Bucket, error) {
	stored, err := c.store.CountsSince(ctx, since)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	pending := c.bucketsLocked(false)
	c.mu.Unlock()
	merged := map[key]int{}
	for _, b := range append(stored, pending...) {
		if !b.Minute.Before(since.Truncate(time.Minute)) {
			merged[key{b.Name, b.Minute.Unix()}] += b.Count
		}
	}
	out := make([]Bucket, 0, len(merged))
	for _, k := range slices.SortedFunc(maps.Keys(merged), func(a, b key) int {
		return strings.Compare(a.name, b.name)*2 + cmpInt(a.minute, b.minute)
	}) {
		out = append(out, Bucket{Name: k.name, Minute: time.Unix(k.minute, 0).UTC(), Count: merged[k]})
	}
	return out, nil
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Series is one metric over the last hour and day.
type Series struct {
	Metric
	LastHour, LastDay int
	Minutes           []int // the last 60 minutes, oldest first, the current minute last
}

// Max is the largest per-minute count, for scaling a chart.
func (s Series) Max() int { return slices.Max(append([]int{0}, s.Minutes...)) }

// Summary returns every Known metric's Series.
func (c *Collector) Summary(ctx context.Context) ([]Series, error) {
	now := c.now().Truncate(time.Minute)
	buckets, err := c.Since(ctx, now.Add(-24*time.Hour+time.Minute))
	if err != nil {
		return nil, err
	}
	out := make([]Series, len(Known))
	index := map[string]int{}
	for i, m := range Known {
		out[i] = Series{Metric: m, Minutes: make([]int, 60)}
		index[m.Name] = i
	}
	for _, b := range buckets {
		i, ok := index[b.Name]
		if !ok {
			continue
		}
		out[i].LastDay += b.Count
		if ago := int(now.Sub(b.Minute) / time.Minute); ago >= 0 && ago < 60 {
			out[i].LastHour += b.Count
			out[i].Minutes[59-ago] += b.Count
		}
	}
	return out, nil
}

// Transport counts the GitHub API traffic going through base, and logs
// each failed request with what GitHub said about it.
func Transport(base http.RoundTripper, c *Collector) http.RoundTripper {
	return roundTripper{base: base, c: c}
}

type roundTripper struct {
	base http.RoundTripper
	c    *Collector
}

func (t roundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	t.c.Inc(GitHubRequests)
	if strings.HasSuffix(r.URL.Path, "/graphql") {
		t.c.Inc(GitHubGraphQL)
	}
	start := time.Now()
	resp, err := t.base.RoundTrip(r)
	switch {
	case err != nil && errors.Is(err, context.Canceled):
		// The caller gave up (e.g. the browser left the page): not GitHub's
		// failure. Timeouts (DeadlineExceeded) still count.
		slog.Debug("GitHub request cancelled", "method", r.Method, "path", r.URL.RequestURI())
	case err != nil:
		t.c.Inc(GitHubErrors)
		slog.Warn("GitHub request failed", "method", r.Method, "path", r.URL.RequestURI(),
			"err", err, "duration", time.Since(start).Round(time.Millisecond))
	case resp.StatusCode == http.StatusNotModified:
		t.c.Inc(GitHubNotModified)
	case resp.StatusCode >= 400:
		t.c.Inc(GitHubErrors)
		limited := resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"
		if limited {
			t.c.Inc(GitHubRateLimited)
		}
		slog.Warn("GitHub request failed", "method", r.Method, "path", r.URL.RequestURI(),
			"status", resp.StatusCode, "rate_limited", limited,
			"rate_limit_remaining", resp.Header.Get("X-RateLimit-Remaining"),
			"request_id", resp.Header.Get("X-GitHub-Request-Id"),
			"duration", time.Since(start).Round(time.Millisecond))
	}
	return resp, err
}
