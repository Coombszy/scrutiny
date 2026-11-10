package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

// withDiff gives a.go a diff (between from and to) of one context line,
// one deleted line (old 2) and one added line (new 2).
func (f *fixture) withDiff(from, to domain.SHA) domain.FileChange {
	change := domain.FileChange{Path: "a.go", OldPath: "a.go", Status: domain.Modified, Additions: 1, Deletions: 1}
	f.repos.ChangeSets[repostore.Pair{From: from, To: to}] = []domain.FileChange{change}
	f.repos.Diffs[repostore.Pair{From: from, To: to}] = map[string]domain.FileDiff{"a.go": {
		FileChange: change,
		Hunks: []domain.Hunk{{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 2, Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 1, NewNo: 1, Text: "package a"},
			{Kind: domain.Delete, OldNo: 2, Text: "var x = 1"},
			{Kind: domain.Add, NewNo: 2, Text: "var x = 2"},
		}}},
	}}
	return change
}

func threadIDs(ts []domain.ReviewThread) []string {
	var ids []string
	for _, t := range ts {
		ids = append(ids, t.ID)
	}
	return ids
}

func TestThreadsArePlacedOnTheirLinesInTheDefaultComparison(t *testing.T) {
	f := newFixture(t)
	change := f.withDiff(mb, c3)
	f.host.State(ref).Threads = []domain.ReviewThread{
		{ID: "right", Path: "a.go", Line: 2, Side: domain.Right},
		{ID: "left", Path: "a.go", Line: 2, Side: domain.Left},
		{ID: "context", Path: "a.go", Line: 1, Side: domain.Right},
		{ID: "outdated", Path: "a.go", Outdated: true},
		{ID: "off-diff", Path: "a.go", Line: 40, Side: domain.Right},
		{ID: "other-file", Path: "b.go", Line: 1, Side: domain.Right},
	}
	ctx := context.Background()
	page, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Files[0].Threads != 5 {
		t.Errorf("a.go thread count = %d, want 5", page.Files[0].Threads)
	}

	d, err := f.svc.FileDiff(ctx, ref, mb, c3, change)
	if err != nil {
		t.Fatal(err)
	}
	lines := d.Hunks[0].Lines
	for i, want := range [][]string{{"context"}, {"left"}, {"right"}} {
		if got := threadIDs(lines[i].Threads); len(got) != 1 || got[0] != want[0] {
			t.Errorf("line %d threads = %v, want %v", i, got, want)
		}
	}
	if got := threadIDs(d.Unplaced); len(got) != 2 || got[0] != "outdated" || got[1] != "off-diff" {
		t.Errorf("Unplaced = %v, want [outdated off-diff]", got)
	}
}

func TestThreadsAreUnplacedInAComparisonNotEndingAtHead(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	change := f.withDiff(c1, c2)
	f.host.State(ref).Threads = []domain.ReviewThread{{ID: "right", Path: "a.go", Line: 2, Side: domain.Right}}
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{First: c2, Last: c2}); err != nil {
		t.Fatal(err)
	}

	d, err := f.svc.FileDiff(ctx, ref, c1, c2, change)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Hunks[0].Lines[2].Threads) != 0 || len(d.Unplaced) != 1 {
		t.Errorf("line threads %v, unplaced %v; want the thread unplaced", d.Hunks[0].Lines[2].Threads, d.Unplaced)
	}
}

func TestAddingAThreadStartsAPendingReviewAtTheComparisonHead(t *testing.T) {
	f := newFixture(t)
	change := f.withDiff(mb, c3)
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{"first", "second"} {
		_, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: body})
		if err != nil {
			t.Fatal(err)
		}
	}
	st := f.host.State(ref)
	if st.Pending == nil || st.Pending.Commit != c3 || st.Pending.Comments != 2 {
		t.Fatalf("Pending = %+v, want one review at head with 2 comments", st.Pending)
	}

	// The new threads are visible to the next fragment without a page load.
	d, err := f.svc.FileDiff(ctx, ref, mb, c3, change)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Hunks[0].Lines[2].Threads; len(got) != 2 || !got[0].Comments[0].Pending {
		t.Errorf("threads on the added line = %+v", got)
	}
}

func TestCommentingIsRefusedWhereGitHubWouldMisplaceIt(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	f.withDiff(c1, c2)
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	// The left side of a range is not the PR's base.
	_, err := f.svc.AddThread(ctx, ref, c1, c2, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Left, Body: "x"})
	if !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("left side of a range: err = %v, want ErrCannotComment", err)
	}

	// A Pending Review on head cannot take comments made against c2.
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.AddThread(ctx, ref, c1, c2, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"})
	if !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("pending review elsewhere: err = %v, want ErrCannotComment", err)
	}
}

func TestReplyResolveReactAndViewedGoToGitHub(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	st := f.host.State(ref)
	st.Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right, ViewerCanResolve: true,
		Comments: []domain.Comment{{ID: "C1", Body: "hm"}}}}
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	th, err := f.svc.Reply(ctx, ref, "T1", "agreed")
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Comments) != 2 || th.Comments[1].Body != "agreed" || !th.Comments[1].Pending {
		t.Errorf("after reply: %+v", th.Comments)
	}
	if st.Pending == nil || st.Pending.Commit != c3 {
		t.Errorf("reply did not start a pending review at head: %+v", st.Pending)
	}

	if th, err = f.svc.SetResolved(ctx, ref, "T1", true); err != nil || !th.Resolved {
		t.Errorf("resolve: %+v, %v", th, err)
	}

	groups, err := f.svc.SetReaction(ctx, ref, "C1", domain.Rocket, true)
	if err != nil || len(groups) != 1 || groups[0].Content != domain.Rocket || groups[0].Count != 1 {
		t.Errorf("react: %+v, %v", groups, err)
	}
	if _, err := f.svc.SetReaction(ctx, ref, "C1", "SHRUG", true); !errors.Is(err, review.ErrInvalidReaction) {
		t.Errorf("unknown reaction: err = %v", err)
	}

	if err := f.svc.SetViewed(ctx, ref, "a.go", true); err != nil {
		t.Fatal(err)
	}
	page, _ := f.svc.PRPage(ctx, ref, review.ComparisonRequest{})
	if page.Files[0].Viewed != domain.Viewed {
		t.Errorf("Viewed = %q", page.Files[0].Viewed)
	}
}

func TestConversationCommentsArePostedImmediately(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	ctx := context.Background()

	conv, err := f.svc.AddConversationComment(ctx, ref, "Thanks!")
	if err != nil {
		t.Fatal(err)
	}
	if len(conv) != 1 || conv[0].Body != "Thanks!" || conv[0].Pending {
		t.Errorf("Conversation = %+v", conv)
	}
	if f.host.State(ref).Pending != nil {
		t.Error("a Conversation Comment started a Pending Review")
	}
}

func TestSubmittingAReview(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	// Without a Pending Review, the review is created at the Comparison head.
	if err := f.svc.Submit(ctx, ref, c3, domain.Approve, "LGTM"); err != nil {
		t.Fatal(err)
	}
	// With one, it is submitted at its own commit.
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Submit(ctx, ref, c2, domain.RequestChanges, "fix it"); err != nil {
		t.Fatal(err)
	}

	want := []codehost.Submission{
		{PR: ref, Commit: c3, Event: domain.Approve, Body: "LGTM"},
		{PR: ref, Commit: c3, Event: domain.RequestChanges, Body: "fix it"},
	}
	if len(f.host.Submissions) != 2 || f.host.Submissions[0] != want[0] || f.host.Submissions[1] != want[1] {
		t.Errorf("Submissions = %+v", f.host.Submissions)
	}
	if f.host.State(ref).Pending != nil {
		t.Error("Pending Review still open after submit")
	}
	if err := f.svc.Submit(ctx, ref, c3, "MAYBE", ""); !errors.Is(err, review.ErrInvalidEvent) {
		t.Errorf("unknown event: err = %v", err)
	}
}
