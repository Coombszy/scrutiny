package review_test

import (
	"context"
	"slices"
	"testing"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

func TestForgettingAPRDropsItsVisitAndLocalData(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = nil
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	f.repos.Worktrees[ref] = c3
	other := domain.PRRef{Repo: repo, Number: 70}
	f.cache.RecordVisit(ctx, cache.Visit{PR: other, Title: "Keep me", VisitedAt: f.now})
	f.cache.Put(ctx, "review:"+other.String(), cache.Entry{Value: []byte("{}")})
	f.cache.Put(ctx, "mergebase:"+other.String()+":main@"+string(c3), cache.Entry{Value: []byte(mb)})
	f.cache.Put(ctx, "dashboard", cache.Entry{Value: []byte("{}")})
	before := f.cache.Keys()
	if !slices.Contains(before, "review:"+ref.String()) || !slices.ContainsFunc(before, func(k string) bool { return k > "mergebase:"+ref.String()+":" && k < "mergebase:"+ref.String()+";" }) {
		t.Fatalf("opening the PR cached %q; want its snapshot and merge base", before)
	}

	if err := f.svc.ForgetPR(ctx, ref); err != nil {
		t.Fatal(err)
	}

	visits, _ := f.cache.RecentVisits(ctx, 10)
	if len(visits) != 1 || visits[0].PR != other {
		t.Errorf("visits = %+v, want only #70", visits)
	}
	want := []string{"dashboard", "mergebase:" + other.String() + ":main@" + string(c3), "review:" + other.String()}
	if got := f.cache.Keys(); !slices.Equal(got, want) {
		t.Errorf("cache keys = %q (before %q), want %q", got, before, want)
	}
	if _, ok := f.repos.Worktrees[ref]; ok {
		t.Error("the PR's worktree is still there")
	}
	if !slices.Equal(f.host.Forgotten, []domain.PRRef{ref}) {
		t.Errorf("code host forgot %v", f.host.Forgotten)
	}
}
