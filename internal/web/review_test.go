package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

const prPath = "/github.com/o/r/pull/7"

func (a *app) withThread() {
	st := a.host.State(ref)
	st.Threads = []domain.ReviewThread{{
		ID: "T1", Path: "parser.go", Line: 2, Side: domain.Right, ViewerCanResolve: true,
		Comments: []domain.Comment{{ID: "C1", Author: "pat", BodyHTML: "<p>Why <em>renewed</em>?</p>",
			Reactions: []domain.ReactionGroup{{Content: domain.Eyes, Count: 2}}}},
	}}
}

func TestPRPageShowsReviewState(t *testing.T) {
	a := newApp(t)
	a.withThread()
	st := a.host.State(ref)
	st.Conversation = []domain.Comment{{ID: "IC1", Author: "sam", BodyHTML: "<p>Nice work</p>"}}
	st.Viewed["go.sum"] = domain.Viewed

	rec := a.do(t, http.MethodGet, prPath, nil)
	body := rec.Body.String()
	mustContain(t, body,
		"Conversation", "<p>Nice work</p>", `hx-post="/github.com/o/r/pull/7/comments"`,
		`id="submit-review"`, `value="APPROVE"`, `value="REQUEST_CHANGES"`, `value="COMMENT"`,
		`data-threads="1"`, // parser.go
		`hx-post="/github.com/o/r/pull/7/viewed"`,
	)
	if !strings.Contains(body, `class="file viewed"`) {
		t.Error("viewed go.sum is not marked viewed")
	}
	if h := rec.Header().Get("Content-Security-Policy"); !strings.Contains(h, "script-src 'self'") {
		t.Errorf("CSP = %q", h)
	}
}

func TestFileFragmentShowsThreadsWithReplyAndResolve(t *testing.T) {
	a := newApp(t)
	a.withThread()
	a.do(t, http.MethodGet, prPath, nil)

	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()
	mustContain(t, body,
		`id="thread-T1"`, `data-resolved="false"`, "<p>Why <em>renewed</em>?</p>", "pat",
		`hx-post="/github.com/o/r/pull/7/threads/T1/replies"`,
		`hx-post="/github.com/o/r/pull/7/threads/T1/resolve"`,
		"👀", // the reaction
		`data-new-thread="/github.com/o/r/pull/7/threads/new?`, `class="add-comment"`, `data-side="RIGHT" data-line="2"`,
	)
}

func TestNewThreadFormCarriesItsAnchor(t *testing.T) {
	a := newApp(t)
	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"}}
	body := a.do(t, http.MethodGet, prPath+"/threads/new?"+q.Encode(), nil).Body.String()
	mustContain(t, body, `hx-post="/github.com/o/r/pull/7/threads"`, `name="line" value="3"`, `name="side" value="RIGHT"`, `name="head" value="`+string(c2)+`"`, "<textarea")
}

func TestCreatingAThreadReturnsItWithTheNewPendingCount(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)

	rec := a.do(t, http.MethodPost, prPath+"/threads", url.Values{
		"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"}, "body": {"Name this better"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	mustContain(t, rec.Body.String(), "Name this better", "Pending", `id="pending-count" hx-swap-oob="true"`, ">1<")
	if p := a.host.State(ref).Pending; p == nil || p.Commit != c2 {
		t.Errorf("Pending = %+v", p)
	}
}

func TestCommentingWhereItWouldBeMisplacedIsAConflict(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodPost, prPath+"/threads", url.Values{
		"base": {string(c1)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"1"}, "side": {"LEFT"}, "body": {"x"},
	})
	if rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rec.Code)
	}
}

func TestReplyAndResolveReturnTheThread(t *testing.T) {
	a := newApp(t)
	a.withThread()
	a.do(t, http.MethodGet, prPath, nil)

	rec := a.do(t, http.MethodPost, prPath+"/threads/T1/replies", url.Values{"body": {"Because it is new"}})
	mustContain(t, rec.Body.String(), `id="thread-T1"`, "Because it is new", `id="pending-count" hx-swap-oob="true"`)

	rec = a.do(t, http.MethodPost, prPath+"/threads/T1/resolve", url.Values{"resolved": {"true"}})
	mustContain(t, rec.Body.String(), `id="thread-T1"`, `data-resolved="true"`, "Unresolve")
}

func TestReactingReturnsTheReactionBar(t *testing.T) {
	a := newApp(t)
	a.withThread()
	a.do(t, http.MethodGet, prPath, nil)

	rec := a.do(t, http.MethodPost, prPath+"/reactions", url.Values{"subject": {"C1"}, "content": {"EYES"}, "on": {"true"}})
	mustContain(t, rec.Body.String(), `id="reactions-C1"`, "👀", ">3<", "reacted")
	if rec := a.do(t, http.MethodPost, prPath+"/reactions", url.Values{"subject": {"C1"}, "content": {"SHRUG"}, "on": {"true"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown reaction: status %d, want 400", rec.Code)
	}
}

func TestConversationCommentIsPosted(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodPost, prPath+"/comments", url.Values{"body": {"Thanks all"}})
	mustContain(t, rec.Body.String(), `id="conversation"`, "Thanks all")
}

func TestViewedIsSynced(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodPost, prPath+"/viewed", url.Values{"path": {"parser.go"}, "viewed": {"true"}})
	if rec.Code != http.StatusNoContent || a.host.State(ref).Viewed["parser.go"] != domain.Viewed {
		t.Errorf("status %d, viewed %q", rec.Code, a.host.State(ref).Viewed["parser.go"])
	}
}

func TestSubmittingAReviewReloadsThePage(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodPost, prPath+"/review", url.Values{"head": {string(c2)}, "event": {"APPROVE"}, "body": {"LGTM"}})
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("status %d, HX-Refresh %q", rec.Code, rec.Header().Get("HX-Refresh"))
	}
	want := codehost.Submission{PR: ref, Commit: c2, Event: domain.Approve, Body: "LGTM"}
	if len(a.host.Submissions) != 1 || a.host.Submissions[0] != want {
		t.Errorf("Submissions = %+v", a.host.Submissions)
	}
}
