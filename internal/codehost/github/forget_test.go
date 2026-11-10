package github_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Coombszy/scrutiny/internal/cache"
)

func TestForgetDropsOnlyThePRsCachedResponses(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemory()
	pull := "github:/repos/" + pr325.Owner + "/" + pr325.Name + "/pulls/325"
	for _, key := range []string{
		pull, pull + "/commits?per_page=100", pull + "?x=1",
		pull + "0", pull + "0/commits?per_page=100", // PR #3250
		"github:/search/issues?q=is%3Apr", "review:" + pr325.String(),
	} {
		c.Put(ctx, key, cache.Entry{Value: []byte("{}")})
	}
	if err := newClient(fixtureServer(t, nil), c).Forget(ctx, pr325); err != nil {
		t.Fatal(err)
	}
	want := []string{"github:/repos/" + pr325.Owner + "/" + pr325.Name + "/pulls/3250", pull + "0/commits?per_page=100", "github:/search/issues?q=is%3Apr", "review:" + pr325.String()}
	slices.Sort(want)
	if got := c.Keys(); !slices.Equal(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
}
