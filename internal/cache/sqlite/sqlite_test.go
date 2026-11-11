package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/cache/sqlite"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/metrics"
)

func open(t *testing.T, path string) *sqlite.Cache {
	t.Helper()
	c, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestEntriesPersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state", "cache.db")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	c := open(t, path)
	if err := c.Put(ctx, "k", cache.Entry{Value: []byte("v1"), ETag: `"a"`, FetchedAt: at}); err != nil {
		t.Fatal(err)
	}
	if err := c.Put(ctx, "k", cache.Entry{Value: []byte("v2"), ETag: `"b"`, FetchedAt: at}); err != nil {
		t.Fatal(err)
	}
	c.Close()

	got, ok, err := open(t, path).Get(ctx, "k")
	if err != nil || !ok {
		t.Fatalf("Get = ok %v, err %v", ok, err)
	}
	if string(got.Value) != "v2" || got.ETag != `"b"` || !got.FetchedAt.Equal(at) {
		t.Errorf("Get = %+v, want the second Put", got)
	}
}

func TestGetMissingKey(t *testing.T) {
	_, ok, err := open(t, filepath.Join(t.TempDir(), "cache.db")).Get(context.Background(), "nope")
	if err != nil || ok {
		t.Errorf("Get = ok %v, err %v; want not found", ok, err)
	}
}

func TestRecentVisitsAreDedupedAndNewestFirst(t *testing.T) {
	ctx := context.Background()
	c := open(t, filepath.Join(t.TempDir(), "cache.db"))
	repo := domain.Repo{Host: "github.com", Owner: "o", Name: "r"}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, v := range []cache.Visit{
		{PR: domain.PRRef{Repo: repo, Number: 1}, Title: "one", VisitedAt: t0},
		{PR: domain.PRRef{Repo: repo, Number: 2}, Title: "two", VisitedAt: t0.Add(time.Hour)},
		{PR: domain.PRRef{Repo: repo, Number: 1}, Title: "one, renamed", VisitedAt: t0.Add(2 * time.Hour)},
	} {
		if err := c.RecordVisit(ctx, v); err != nil {
			t.Fatalf("visit %d: %v", i, err)
		}
	}

	got, err := c.RecentVisits(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].PR.Number != 1 || got[0].Title != "one, renamed" || got[1].PR.Number != 2 {
		t.Errorf("RecentVisits = %+v", got)
	}
	if got[0].PR.Repo != repo || !got[0].VisitedAt.Equal(t0.Add(2*time.Hour)) {
		t.Errorf("RecentVisits[0] = %+v", got[0])
	}
}

func TestDeletingEntriesAndVisits(t *testing.T) {
	ctx := context.Background()
	c := open(t, filepath.Join(t.TempDir(), "cache.db"))
	for _, key := range []string{"a:1", "a:1/x", "a:10", "a%_", "b"} {
		if err := c.Put(ctx, key, cache.Entry{Value: []byte("v")}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Delete(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeletePrefix(ctx, "a:1/"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeletePrefix(ctx, "a%"); err != nil { // % is literal, not a wildcard
		t.Fatal(err)
	}
	for key, want := range map[string]bool{"a:1": true, "a:1/x": false, "a:10": true, "a%_": false, "b": false} {
		if _, ok, _ := c.Get(ctx, key); ok != want {
			t.Errorf("%q present = %v, want %v", key, ok, want)
		}
	}

	repo := domain.Repo{Host: "github.com", Owner: "o", Name: "r"}
	one, two := domain.PRRef{Repo: repo, Number: 1}, domain.PRRef{Repo: repo, Number: 2}
	c.RecordVisit(ctx, cache.Visit{PR: one, Title: "one"})
	c.RecordVisit(ctx, cache.Visit{PR: two, Title: "two"})
	if err := c.DeleteVisit(ctx, one); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.RecentVisits(ctx, 10); len(got) != 1 || got[0].PR != two {
		t.Errorf("RecentVisits = %+v, want only #2", got)
	}
}

func TestMetricCountsSumPersistAndPrune(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cache.db")
	m0 := time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC)
	c := open(t, path)
	var _ metrics.Store = c
	if err := c.AddCounts(ctx, []metrics.Bucket{{Name: "a", Minute: m0, Count: 2}, {Name: "a", Minute: m0.Add(time.Minute), Count: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := c.AddCounts(ctx, []metrics.Bucket{{Name: "a", Minute: m0, Count: 3}}); err != nil {
		t.Fatal(err)
	}
	c.Close()

	c = open(t, path)
	got, err := c.CountsSince(ctx, m0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Count != 5 || !got[0].Minute.Equal(m0) || got[1].Count != 1 {
		t.Errorf("CountsSince = %+v", got)
	}
	if err := c.PruneCounts(ctx, m0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.CountsSince(ctx, m0); len(got) != 1 || !got[0].Minute.Equal(m0.Add(time.Minute)) {
		t.Errorf("after pruning: %+v", got)
	}
}
