package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/codehost/github"
	"github.com/Coombszy/scrutiny/internal/domain"
)

var pr325 = domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: "cli", Name: "go-gh"}, Number: 325}

// fixtureServer serves recorded GitHub API responses keyed by request path.
func fixtureServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		b, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newClient(srv *httptest.Server, c cache.Cache) *github.Client {
	return github.New(srv.Client(), srv.URL, c)
}

func TestPullRequestMapsMetadata(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/repos/cli/go-gh/pulls/325": "pull.json"})

	got, err := newClient(srv, cache.NewMemory()).PullRequest(context.Background(), pr325)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.PullRequest{
		Ref:          pr325,
		Title:        "Fix auth token forwarding for hosts with ports",
		Author:       "eeroniemi",
		State:        domain.Closed,
		BaseBranch:   "trunk",
		HeadBranch:   "auth-token-port",
		BaseSHA:      "4e83b33c301de3d1d4d30df34db06648f898c389",
		HeadSHA:      "15fbe1db354aaa7ac0d966e5099f4c0064d4db02",
		URL:          "https://github.com/cli/go-gh/pull/325",
		Additions:    19,
		ChangedFiles: 2,
		UpdatedAt:    time.Date(2026, 11, 10, 15, 20, 0, 0, time.UTC),
	}
	got.Body = ""
	if got != want {
		t.Errorf("PullRequest =\n%+v\nwant\n%+v", got, want)
	}
}

func TestPullRequestNotFound(t *testing.T) {
	srv := fixtureServer(t, nil)
	_, err := newClient(srv, cache.NewMemory()).PullRequest(context.Background(), pr325)
	if !errors.Is(err, codehost.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestRepeatRequestsRevalidateWithETag(t *testing.T) {
	body, err := os.ReadFile("testdata/pull.json")
	if err != nil {
		t.Fatal(err)
	}
	var full, notModified atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"v1"` {
			notModified.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		full.Add(1)
		w.Header().Set("ETag", `"v1"`)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	client := newClient(srv, cache.NewMemory())

	for range 3 {
		got, err := client.PullRequest(context.Background(), pr325)
		if err != nil {
			t.Fatal(err)
		}
		if got.HeadSHA != "15fbe1db354aaa7ac0d966e5099f4c0064d4db02" {
			t.Fatalf("HeadSHA = %s", got.HeadSHA)
		}
	}
	if full.Load() != 1 || notModified.Load() != 2 {
		t.Errorf("full responses = %d, 304s = %d; want 1 and 2", full.Load(), notModified.Load())
	}
}

func TestCommitsFollowsPagination(t *testing.T) {
	body, err := os.ReadFile("testdata/commits.json")
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `<`+srv.URL+`/repos/cli/go-gh/pulls/325/commits?per_page=100&page=2>; rel="next"`)
		}
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	got, err := newClient(srv, cache.NewMemory()).Commits(context.Background(), pr325)
	if err != nil {
		t.Fatal(err)
	}
	want := domain.Commit{
		SHA:        "15fbe1db354aaa7ac0d966e5099f4c0064d4db02",
		Headline:   "Fix auth token forwarding for hosts with ports",
		Author:     "eeroniemi",
		AuthoredAt: time.Date(2026, 11, 10, 13, 57, 2, 0, time.UTC),
	}
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Errorf("Commits = %+v, want two copies of %+v", got, want)
	}
}

func TestDashboardSearchesReviewRequestsAndAuthored(t *testing.T) {
	var (
		mu      sync.Mutex // the two searches run in parallel
		queries []string
	)
	body, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Query().Get("q"))
		mu.Unlock()
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	got, err := newClient(srv, cache.NewMemory()).Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(queries)
	if len(queries) != 2 || queries[0] != "is:pr is:open archived:false author:@me" || queries[1] != "is:pr is:open archived:false review-requested:@me" {
		t.Errorf("queries = %q", queries)
	}
	want := domain.PRSummary{
		Ref:       domain.PRRef{Repo: pr325.Repo, Number: 324},
		Title:     "chore(deps): Bump golang.org/x/sys from 0.49.0 to 0.50.0",
		Author:    "dependabot[bot]",
		UpdatedAt: time.Date(2026, 11, 9, 23, 42, 51, 0, time.UTC),
	}
	if len(got.ReviewRequested) != 3 || got.ReviewRequested[0] != want || len(got.Authored) != 3 {
		t.Errorf("Dashboard = %+v", got)
	}
}

func TestDashboardSearchesRunInParallel(t *testing.T) {
	body, err := os.ReadFile("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	var inFlight, peak atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := inFlight.Add(1); n > peak.Load() {
			peak.Store(n)
		}
		if inFlight.Load() == 2 {
			close(release)
		}
		select {
		case <-release:
		case <-time.After(2 * time.Second):
		}
		inFlight.Add(-1)
		w.Write(body)
	}))
	t.Cleanup(srv.Close)

	if _, err := newClient(srv, cache.NewMemory()).Dashboard(context.Background()); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 {
		t.Errorf("at most %d search in flight, want both at once", peak.Load())
	}
}
