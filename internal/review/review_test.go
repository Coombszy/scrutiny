package review_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/settings"
)

var (
	repo = domain.Repo{Host: "github.com", Owner: "o", Name: "r"}
	ref  = domain.PRRef{Repo: repo, Number: 7}
)

// Commits: mb (merge base) <- c1 <- c2 <- c3 (head); main's tip is tip.
const (
	tip domain.SHA = "tip0000000000000000000000000000000000000"
	mb  domain.SHA = "mb00000000000000000000000000000000000000"
	c1  domain.SHA = "c100000000000000000000000000000000000000"
	c2  domain.SHA = "c200000000000000000000000000000000000000"
	c3  domain.SHA = "c300000000000000000000000000000000000000"
)

type fixture struct {
	host     *codehost.Fake
	repos    *repostore.Fake
	cache    *cache.Memory
	settings *settings.Memory
	nav      *navigator.Fake
	svc      *review.Service
	now      time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{
		host:     codehost.NewFake(),
		repos:    repostore.NewFake(),
		cache:    cache.NewMemory(),
		settings: settings.NewMemory(settings.Defaults()),
		nav:      navigator.NewFake(),
		now:      time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC),
	}
	f.host.PRs[ref] = domain.PullRequest{Ref: ref, Title: "Add things", BaseBranch: "main", HeadSHA: c3}
	f.host.CommitsOf[ref] = []domain.Commit{{SHA: c1}, {SHA: c2}, {SHA: c3}}
	f.repos.Remotes[ref] = repostore.FakeRemote{BaseTip: tip, Commits: []domain.SHA{c1, c2, c3}}
	f.repos.MergeBases[repostore.Pair{From: tip, To: c3}] = mb
	f.svc = review.NewService(review.Deps{
		CodeHost:    f.host,
		Repos:       f.repos,
		Highlighter: highlight.PlainHighlighter{},
		Cache:       f.cache,
		Settings:    f.settings,
		Navigator:   f.nav,
		Now:         func() time.Time { return f.now },
	})
	return f
}

func TestDefaultComparisonIsMergeBaseToHead(t *testing.T) {
	f := newFixture(t)
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = []domain.FileChange{
		{Path: "a.go", OldPath: "a.go", Status: domain.Modified, Additions: 3},
	}

	page, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Comparison.Base != mb || page.Comparison.Head != c3 || !page.Comparison.IsDefault() {
		t.Errorf("Comparison = %+v, want default %s..%s", page.Comparison, mb.Short(), c3.Short())
	}
	if len(page.Files) != 1 || page.Files[0].Path != "a.go" {
		t.Errorf("Files = %+v", page.Files)
	}
	if page.PR.Title != "Add things" || len(page.Commits) != 3 {
		t.Errorf("page = %+v", page)
	}
}

func TestCommitRangeComparesParentOfFirstToLast(t *testing.T) {
	f := newFixture(t)
	f.repos.ChangeSets[repostore.Pair{From: c1, To: c3}] = nil
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c2}] = nil

	page, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{First: c2, Last: c3})
	if err != nil {
		t.Fatal(err)
	}
	if page.Comparison.Base != c1 || page.Comparison.Head != c3 || page.Comparison.IsDefault() {
		t.Errorf("c2..c3: Comparison = %+v, want %s..%s", page.Comparison, c1.Short(), c3.Short())
	}

	page, err = f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{First: c1, Last: c2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Comparison.Base != mb || page.Comparison.Head != c2 {
		t.Errorf("c1..c2: Comparison = %+v, want %s..%s", page.Comparison, mb.Short(), c2.Short())
	}
}

func TestInvalidCommitRangeIsRejected(t *testing.T) {
	f := newFixture(t)
	for name, req := range map[string]review.ComparisonRequest{
		"unknown commit": {First: "nope", Last: c3},
		"reversed":       {First: c3, Last: c1},
		"only one end":   {First: c1},
	} {
		_, err := f.svc.PRPage(context.Background(), ref, req)
		if !errors.Is(err, review.ErrInvalidComparison) {
			t.Errorf("%s: err = %v, want ErrInvalidComparison", name, err)
		}
	}
}

func TestReopeningAPRWithAKnownHeadDoesNotFetch(t *testing.T) {
	f := newFixture(t)
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = nil
	ctx := context.Background()

	for range 2 {
		if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if f.repos.Fetches != 1 {
		t.Errorf("fetches = %d, want 1", f.repos.Fetches)
	}

	// A new head is fetched.
	const c4 domain.SHA = "c400000000000000000000000000000000000000"
	f.host.PRs[ref] = domain.PullRequest{Ref: ref, BaseBranch: "main", HeadSHA: c4}
	f.repos.Remotes[ref] = repostore.FakeRemote{BaseTip: tip, Commits: []domain.SHA{c1, c2, c3, c4}}
	f.repos.MergeBases[repostore.Pair{From: tip, To: c4}] = mb
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c4}] = nil
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	if f.repos.Fetches != 2 {
		t.Errorf("fetches after head moved = %d, want 2", f.repos.Fetches)
	}
}

func TestOpeningAPRRecordsAVisit(t *testing.T) {
	f := newFixture(t)
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = nil

	if _, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	visits, _ := f.cache.RecentVisits(context.Background(), 10)
	if len(visits) != 1 || visits[0].PR != ref || visits[0].Title != "Add things" || !visits[0].VisitedAt.Equal(f.now) {
		t.Errorf("visits = %+v", visits)
	}
}

func TestLargeAndGeneratedFilesStartCollapsed(t *testing.T) {
	f := newFixture(t)
	cfg := settings.Defaults()
	cfg.Diff.CollapseThreshold = 100
	f.settings.Save(cfg)
	f.repos.Attrs["linguist-generated"] = map[string]string{"gen/api.go": "set", "package-lock.json": "false"}
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = []domain.FileChange{
		{Path: "small.go", Additions: 60, Deletions: 40},
		{Path: "big.go", Additions: 60, Deletions: 41},
		{Path: "gen/api.go", Additions: 1},
		{Path: "web/yarn.lock", Additions: 1},
		{Path: "package-lock.json", Additions: 1},
		{Path: "logo.png", Binary: true},
	}

	page, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]review.Collapse{
		"small.go":          review.Expanded,
		"big.go":            review.CollapsedLarge,
		"gen/api.go":        review.CollapsedGenerated,
		"web/yarn.lock":     review.CollapsedGenerated,
		"package-lock.json": review.Expanded, // linguist-generated=false wins
		"logo.png":          review.Expanded,
	}
	for _, file := range page.Files {
		if file.Collapse != want[file.Path] {
			t.Errorf("%s: Collapse = %q, want %q", file.Path, file.Collapse, want[file.Path])
		}
	}
}

func TestDashboardCombinesCodeHostListsWithRecentVisits(t *testing.T) {
	f := newFixture(t)
	other := domain.PRRef{Repo: repo, Number: 9}
	f.host.Dash = codehost.Dashboard{
		ReviewRequested: []domain.PRSummary{{Ref: other, Title: "Please review"}},
		Authored:        []domain.PRSummary{{Ref: ref, Title: "Mine"}},
	}
	f.cache.RecordVisit(context.Background(), cache.Visit{PR: ref, Title: "Mine", VisitedAt: f.now})

	got, err := f.svc.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ReviewRequested) != 1 || len(got.Authored) != 1 || len(got.Recent) != 1 || got.Recent[0].PR != ref {
		t.Errorf("Dashboard = %+v", got)
	}
}

func TestRetargetingAPRRecomputesTheMergeBase(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = nil
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	const otherTip, otherMB domain.SHA = "ot00000000000000000000000000000000000000", "om00000000000000000000000000000000000000"
	f.host.PRs[ref] = domain.PullRequest{Ref: ref, BaseBranch: "release", HeadSHA: c3}
	f.repos.Remotes[ref] = repostore.FakeRemote{BaseTip: otherTip, Commits: []domain.SHA{c1, c2, c3}}
	f.repos.MergeBases[repostore.Pair{From: otherTip, To: c3}] = otherMB
	f.repos.ChangeSets[repostore.Pair{From: otherMB, To: c3}] = nil

	page, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Comparison.Base != otherMB {
		t.Errorf("Base = %s, want the merge base with the new base branch", page.Comparison.Base.Short())
	}
}
