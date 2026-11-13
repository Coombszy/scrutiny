package review_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
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

	d, err := f.svc.FileDiff(ctx, ref, mb, c3, change, false)
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

	d, err := f.svc.FileDiff(ctx, ref, c1, c2, change, false)
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
		_, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: body}, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	st := f.host.State(ref)
	if st.Pending == nil || st.Pending.Commit != c3 || st.Pending.Comments != 2 {
		t.Fatalf("Pending = %+v, want one review at head with 2 comments", st.Pending)
	}

	// The new threads are visible to the next fragment without a page load.
	d, err := f.svc.FileDiff(ctx, ref, mb, c3, change, false)
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
	_, err := f.svc.AddThread(ctx, ref, c1, c2, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Left, Body: "x"}, true)
	if !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("left side of a range: err = %v, want ErrCannotComment", err)
	}

	// A Pending Review on head cannot take comments made against c2.
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true); err != nil {
		t.Fatal(err)
	}
	_, err = f.svc.AddThread(ctx, ref, c1, c2, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true)
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

	th, err := f.svc.Reply(ctx, ref, "T1", "agreed", true)
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
	if len(conv.Comments) != 1 || conv.Comments[0].Body != "Thanks!" || conv.Comments[0].Pending {
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
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true); err != nil {
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

func TestAFailedWriteDoesNotLeaveAStaleSnapshot(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	// The Pending Review is created, then adding the thread fails.
	f.host.FailAddThread = errors.New("line could not be resolved")
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true); err == nil {
		t.Fatal("AddThread succeeded, want the injected failure")
	}
	f.host.FailAddThread = nil

	// The retry must reuse that Pending Review rather than start another.
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestAddThreadReturnsOnlyTheNewThread(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	f.host.State(ref).Threads = []domain.ReviewThread{{ID: "old", Path: "a.go", Line: 2, Side: domain.Right}}
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	got, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID == "old" {
		t.Errorf("AddThread returned %v, want only the new thread", threadIDs(got))
	}
}

func TestWritesAreRefusedForThingsNotOnThisPR(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "not-in-diff.go", Line: 1, Side: domain.Right, Body: "x"}, true); !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("file outside the diff: err = %v, want ErrCannotComment", err)
	}
	if _, err := f.svc.SetResolved(ctx, ref, "PRRT_elsewhere", true); !errors.Is(err, review.ErrThreadNotFound) {
		t.Errorf("foreign thread: err = %v, want ErrThreadNotFound", err)
	}
	if _, err := f.svc.SetReaction(ctx, ref, "IC_elsewhere", domain.Heart, true); !errors.Is(err, review.ErrThreadNotFound) {
		t.Errorf("foreign comment: err = %v, want ErrThreadNotFound", err)
	}
}

func TestPendingCommentsListEveryDraftInFileOrder(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	st := f.host.State(ref)
	st.Threads = []domain.ReviewThread{
		{ID: "T2", Path: "b.go", Line: 9, Side: domain.Right, Comments: []domain.Comment{
			{ID: "C1", Body: "submitted earlier"},
			{ID: "C2", Body: "my draft reply", Pending: true},
		}},
		{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right, Comments: []domain.Comment{{ID: "C3", Body: "new thread", Pending: true}}},
		{ID: "T3", Path: "a.go", Line: 1, Side: domain.Right, Comments: []domain.Comment{{ID: "C4", Body: "nothing pending"}}},
	}
	if _, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	got, err := f.svc.PendingComments(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	want := []review.PendingComment{
		{ThreadID: "T1", Path: "a.go", Line: 2, Body: "new thread"},
		{ThreadID: "T2", Path: "b.go", Line: 9, Body: "my draft reply", Reply: true},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("PendingComments = %+v\nwant %+v", got, want)
	}
}

func TestSingleCommentsArePostedWithoutAReview(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	f.withDiff(mb, c2)
	st := f.host.State(ref)
	st.Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right,
		Comments: []domain.Comment{{ID: "C1", Body: "hm"}}}}
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	th, err := f.svc.Reply(ctx, ref, "T1", "agreed", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(th.Comments) != 2 || th.Comments[1].Pending {
		t.Errorf("single reply: %+v", th.Comments)
	}
	added, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "now"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0].Comments[0].Pending {
		t.Errorf("single thread: %+v", added)
	}
	if st.Pending != nil {
		t.Errorf("a Single Comment started a Pending Review: %+v", st.Pending)
	}

	// A Single Comment goes on the PR's head, so not on an earlier Comparison.
	if _, err := f.svc.AddThread(ctx, ref, mb, c2, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, false); !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("single comment before head: err = %v, want ErrCannotComment", err)
	}
}

func TestSingleCommentsAreRefusedWithAPendingReview(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	st := f.host.State(ref)
	st.Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right,
		Comments: []domain.Comment{{ID: "C1", Body: "hm"}}}}
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Reply(ctx, ref, "T1", "in review", true); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.Reply(ctx, ref, "T1", "now", false); !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("single reply: err = %v, want ErrCannotComment", err)
	}
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "now"}, false); !errors.Is(err, review.ErrCannotComment) {
		t.Errorf("single thread: err = %v, want ErrCannotComment", err)
	}
}

func TestDiscardingAPendingReviewDeletesItsComments(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	st := f.host.State(ref)
	st.Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right,
		Comments: []domain.Comment{{ID: "C1", Body: "hm"}}}}
	ctx := context.Background()
	if _, err := f.svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Discard(ctx, ref); !errors.Is(err, review.ErrNoPendingReview) {
		t.Errorf("no pending review: err = %v, want ErrNoPendingReview", err)
	}

	if _, err := f.svc.Reply(ctx, ref, "T1", "draft reply", true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "draft thread"}, true); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Discard(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if st.Pending != nil {
		t.Errorf("Pending = %+v, want none", st.Pending)
	}
	if got := threadIDs(st.Threads); len(got) != 1 || got[0] != "T1" {
		t.Errorf("threads = %v, want only T1", got)
	}
	if cs := st.Threads[0].Comments; len(cs) != 1 || cs[0].ID != "C1" {
		t.Errorf("T1 comments = %+v, want only C1", cs)
	}
}

// laggingHost keeps returning the review state from before a discard for
// stale more reads, as GitHub does for a few seconds.
type laggingHost struct {
	*codehost.Fake
	stale  int
	before domain.ReviewState
}

func (h *laggingHost) DiscardReview(ctx context.Context, reviewID string) error {
	before, err := h.Fake.ReviewState(ctx, ref)
	if err != nil {
		return err
	}
	h.before = before
	return h.Fake.DiscardReview(ctx, reviewID)
}

func (h *laggingHost) ReviewState(ctx context.Context, r domain.PRRef) (domain.ReviewState, error) {
	if h.before.PRID != "" && h.stale > 0 {
		h.stale--
		return h.before, nil
	}
	return h.Fake.ReviewState(ctx, r)
}

func TestDiscardRereadsWithBackoffUntilGitHubCatchesUp(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stale  int
		sleeps int
	}{
		{"caught up at once", 0, 0},
		{"caught up after two retries", 2, 2},
		{"never caught up", 10, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.withDiff(mb, c3)
			host := &laggingHost{Fake: f.host}
			var sleeps []time.Duration
			svc := review.NewService(review.Deps{
				CodeHost: host, Repos: f.repos, Highlighter: highlight.PlainHighlighter{}, Cache: f.cache, Settings: f.settings,
				Now:        func() time.Time { return f.now },
				Sleep:      func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil },
				Background: func(fn func()) { fn() },
			})
			ctx := context.Background()
			if _, err := svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "draft"}, true); err != nil {
				t.Fatal(err)
			}

			host.stale = tc.stale
			if err := svc.Discard(ctx, ref); err != nil {
				t.Fatal(err)
			}
			if len(sleeps) != tc.sleeps {
				t.Errorf("slept %v, want %d sleeps", sleeps, tc.sleeps)
			}
			if tc.sleeps > 1 && sleeps[1] <= sleeps[0] {
				t.Errorf("sleeps %v do not back off", sleeps)
			}
		})
	}
}

func TestDiscardStopsWaitingWhenTheRequestIsCancelled(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	host := &laggingHost{Fake: f.host}
	ctx, cancel := context.WithCancel(context.Background())
	svc := review.NewService(review.Deps{
		CodeHost: host, Repos: f.repos, Highlighter: highlight.PlainHighlighter{}, Cache: f.cache, Settings: f.settings,
		Now:        func() time.Time { return f.now },
		Sleep:      func(ctx context.Context, _ time.Duration) error { cancel(); return ctx.Err() },
		Background: func(fn func()) { fn() },
	})
	if _, err := svc.PRPage(ctx, ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "draft"}, true); err != nil {
		t.Fatal(err)
	}
	host.stale = 10
	if err := svc.Discard(ctx, ref); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
