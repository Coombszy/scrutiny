package review_test

import (
	"context"
	"testing"

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
	if u.NewThreads != 1 || !u.ConversationChanged || len(u.Conversation) != 1 {
		t.Errorf("NewThreads %d, ConversationChanged %v", u.NewThreads, u.ConversationChanged)
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
