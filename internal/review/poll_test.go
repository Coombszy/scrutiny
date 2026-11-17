package review_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

func (f *fixture) openPage(t *testing.T) review.PRPage {
	t.Helper()
	f.withDiff(mb, c3)
	page, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestPollWithNothingNewIsEmpty(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !u.Empty() {
		t.Errorf("Update = %+v, want empty", u)
	}
}

func TestPollReportsANewHead(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	const c4 domain.SHA = "c400000000000000000000000000000000000000"
	f.host.PRs[ref] = domain.PullRequest{Ref: ref, BaseBranch: "main", HeadSHA: c4}
	f.repos.Remotes[ref] = repostore.FakeRemote{BaseTip: tip, Commits: []domain.SHA{c1, c2, c3, c4}}
	f.repos.MergeBases[repostore.Pair{From: tip, To: c4}] = mb

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if u.Head != c4 {
		t.Errorf("Head = %q, want %s", u.Head, c4.Short())
	}
	if u, _ := f.svc.Poll(context.Background(), ref); !u.Empty() {
		t.Errorf("second poll = %+v, want empty", u)
	}
}

func TestPollReportsChangedAndNewThreads(t *testing.T) {
	f := newFixture(t)
	st := f.host.State(ref)
	st.Threads = []domain.ReviewThread{
		{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right, Comments: []domain.Comment{{ID: "C1", Body: "hm"}}},
		{ID: "T2", Path: "a.go", Line: 1, Side: domain.Right, Comments: []domain.Comment{{ID: "C2", Body: "ok"}}},
	}
	f.openPage(t)

	st.Threads[0].Comments = append(st.Threads[0].Comments, domain.Comment{ID: "C3", Body: "because"})
	st.Threads = append(st.Threads, domain.ReviewThread{ID: "T3", Path: "a.go", Line: 2, Side: domain.Right})
	st.Conversation = []domain.Comment{{ID: "IC1", Body: "hello"}}

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Threads) != 1 || u.Threads[0].ID != "T1" || len(u.Threads[0].Comments) != 2 {
		t.Errorf("Threads = %+v, want just T1 with its reply", u.Threads)
	}
	if u.NewThreads != 1 || !u.ConversationChanged || len(u.Conversation.Comments) != 1 {
		t.Errorf("NewThreads %d, ConversationChanged %v", u.NewThreads, u.ConversationChanged)
	}
}

func TestPollReportsAnEditedDescription(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	f.host.State(ref).Description = domain.Description{
		Author: "sam", BodyHTML: "<p>New</p>", Original: "Old", HasOriginal: true,
		Edits: []domain.DescriptionEdit{{Editor: "sam", Body: "New"}},
	}

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !u.ConversationChanged || u.Conversation.Description.BodyHTML != "<p>New</p>" || len(u.Conversation.Description.Edits) != 1 {
		t.Errorf("Update = %+v", u)
	}
}

func TestPollReportsChecks(t *testing.T) {
	f := newFixture(t)
	f.host.ChecksOf[ref] = domain.Checks{Head: c3, MergeState: domain.MergeBlocked, Checks: []domain.Check{{Name: "test", State: domain.CheckPending}}}
	page := f.openPage(t)
	if page.Checks.MergeState != domain.MergeBlocked || len(page.Checks.Checks) != 1 {
		t.Fatalf("page Checks = %+v", page.Checks)
	}

	f.host.ChecksOf[ref] = domain.Checks{Head: c3, MergeState: domain.MergeClean, Checks: []domain.Check{{Name: "test", State: domain.CheckSuccess}}}
	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if u.Checks == nil || u.Checks.MergeState != domain.MergeClean {
		t.Errorf("Checks = %+v", u.Checks)
	}
}

func TestOfflinePagesAreServedFromTheLastRead(t *testing.T) {
	f := newFixture(t)
	f.host.State(ref).Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Side: domain.Right}}
	f.openPage(t)
	f.host.SetOnline(false)

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !u.OfflineChanged || !u.Offline {
		t.Errorf("Update = %+v, want offline", u)
	}
	page, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !page.Offline || len(page.Review.Threads) != 1 {
		t.Errorf("Offline %v, threads %d; want the cached review state", page.Offline, len(page.Review.Threads))
	}

	f.host.SetOnline(true)
	if u, _ := f.svc.Poll(context.Background(), ref); !u.OfflineChanged || u.Offline {
		t.Errorf("back online: %+v", u)
	}
}

func TestPollCarriesThePendingCommentsWhenTheyChange(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	st := f.host.State(ref)
	st.Pending = &domain.PendingReview{ID: "R", Commit: c3, Comments: 1}
	st.Threads = []domain.ReviewThread{{ID: "T1", Path: "a.go", Line: 2, Comments: []domain.Comment{{ID: "C1", Body: "from another tab", Pending: true}}}}

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !u.PendingChanged || len(u.PendingComments) != 1 || u.PendingComments[0].Body != "from another tab" {
		t.Errorf("Update = %+v", u)
	}
}

func TestPollReportsNewlySubmittedReviews(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	f.host.State(ref).Reviews = []domain.SubmittedReview{{ID: "R1", Author: "ana", Verdict: domain.ReviewApproved, Body: "Ship it"}}

	u, err := f.svc.Poll(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if !u.ConversationChanged || len(u.Conversation.Reviews) != 1 || u.Conversation.Reviews[0].Verdict != domain.ReviewApproved {
		t.Errorf("Update = %+v", u)
	}
}

// A poll whose read began before a write must not save over the state the
// write re-read: the Pending Review would vanish from the cache, and the
// next poll would announce the viewer's own thread as new.
func TestAPollOverlappingAWriteKeepsTheWrite(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	ctx := context.Background()

	read, release := make(chan struct{}), make(chan struct{})
	var paused atomic.Bool
	f.host.AfterReviewState = func() {
		if paused.CompareAndSwap(false, true) { // only the poll's read pauses
			close(read)
			<-release
		}
	}
	polled := make(chan error)
	go func() {
		_, err := f.svc.Poll(ctx, ref)
		polled <- err
	}()
	<-read

	wrote := make(chan error)
	go func() {
		_, err := f.svc.AddThread(ctx, ref, mb, c3, codehost.NewThread{Path: "a.go", Line: 2, Side: domain.Right, Body: "x"}, true)
		wrote <- err
	}()
	var writeErr error
	writeDone := false
	select {
	case writeErr = <-wrote: // no lock: the write ran during the poll's read
		writeDone = true
	case <-time.After(100 * time.Millisecond): // the write waits for the poll
	}
	close(release)
	if err := <-polled; err != nil {
		t.Fatal(err)
	}
	if !writeDone {
		writeErr = <-wrote
	}
	if writeErr != nil {
		t.Fatal(writeErr)
	}

	if a, err := f.svc.Anchors(ctx, ref); err != nil || a.Pending == nil {
		t.Errorf("cached Pending Review = %+v (err %v), want the one the write started", a.Pending, err)
	}
	if u, err := f.svc.Poll(ctx, ref); err != nil || u.NewThreads != 0 {
		t.Errorf("next poll NewThreads = %d (err %v), want 0 for the viewer's own thread", u.NewThreads, err)
	}
}
