package web_test

import (
	"golang.org/x/net/html"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
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
	if !strings.Contains(body, `class="file viewed collapsed"`) {
		t.Error("viewed go.sum is not marked viewed and collapsed")
	}
	if !strings.Contains(body, `data-file-collapse aria-expanded="false"`) {
		t.Error("viewed go.sum's collapse arrow is not shown collapsed")
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
	mustContain(t, rec.Body.String(), "Name this better", "Pending", `id="pending-count" hx-swap-oob="true"`, "(1 pending)",
		`id="pending-list" hx-swap-oob="true"`, `data-jump-thread="`, "parser.go:3")
	if p := a.host.State(ref).Pending; p == nil || p.Commit != c2 {
		t.Errorf("Pending = %+v", p)
	}
}

func TestDraggingOverLinesCommentsOnTheRange(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	mustContain(t, a.do(t, http.MethodGet, fileURL(), nil).Body.String(), `class="add-comment"`, "drag for several")

	form := a.do(t, http.MethodGet, prPath+"/threads/new?"+url.Values{
		"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "cols": {"4"},
		"side": {"RIGHT"}, "line": {"3"}, "start_side": {"RIGHT"}, "start_line": {"1"},
	}.Encode(), nil).Body.String()
	mustContain(t, form, `name="start_line" value="1"`, `name="start_side" value="RIGHT"`, "parser.go:1-3", "New comment")

	rec := a.do(t, http.MethodPost, prPath+"/threads", url.Values{
		"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"},
		"start_line": {"1"}, "start_side": {"RIGHT"}, "body": {"These three"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	mustContain(t, rec.Body.String(), "These three", "parser.go:1-3")
	th := a.host.State(ref).Threads
	if len(th) != 1 || th[0].StartLine != 1 || th[0].StartSide != domain.Right || th[0].Line != 3 {
		t.Errorf("Threads = %+v", th)
	}
}

func TestRangesMustRunDownTheDiff(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	for _, start := range []url.Values{
		{"start_line": {"3"}, "start_side": {"RIGHT"}}, // a single line
		{"start_line": {"5"}, "start_side": {"RIGHT"}}, // upwards
		{"start_line": {"1"}, "start_side": {"UP"}},
	} {
		form := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"}, "body": {"x"}}
		for k, v := range start {
			form[k] = v
		}
		if rec := a.do(t, http.MethodPost, prPath+"/threads", form); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", start, rec.Code)
		}
	}
	if th := a.host.State(ref).Threads; len(th) != 0 {
		t.Errorf("Threads = %+v", th)
	}
}

func TestCommentingWhereItWouldBeMisplacedIsAConflict(t *testing.T) {
	a := newApp(t)
	a.repos.ChangeSets[repostore.Pair{From: c1, To: c2}] = []domain.FileChange{{Path: "parser.go", OldPath: "parser.go", Status: domain.Modified}}
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
	mustContain(t, rec.Body.String(), `id="thread-T1"`, "Because it is new", `id="pending-count" hx-swap-oob="true"`, "(1 pending)", `data-jump-thread="T1"`)

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
	mustContain(t, rec.Body.String(), `id="conversation"`, "Thanks all", `<span id="conversation-count" hx-swap-oob="true">1</span>`)
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

// formValues is an empty form, so that a.do sends the CSRF token.
func formValues() url.Values { return url.Values{} }

func TestSubmitDialogListsPendingComments(t *testing.T) {
	a := newApp(t)
	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page, `id="pending-count"`, `id="pending-list"`)
	if strings.Contains(page, "pending)") {
		t.Error("the Review button counts pending comments when there are none")
	}

	a.withThread()
	st := a.host.State(ref)
	st.Pending = &domain.PendingReview{ID: "R", Commit: c2, Comments: 2}
	st.Threads[0].Comments = append(st.Threads[0].Comments, domain.Comment{ID: "C9", Body: "A **draft** reply", Pending: true})
	st.Threads = append(st.Threads, domain.ReviewThread{ID: "T2", Path: "lex/new.go", Line: 4, Side: domain.Right,
		Comments: []domain.Comment{{ID: "C10", Body: "New thread here", Pending: true}}})

	page = a.do(t, http.MethodGet, prPath, nil).Body.String()
	dialog := page[strings.Index(page, `id="submit-review"`):]
	dialog = dialog[:strings.Index(dialog, "</dialog>")]
	mustContain(t, page, "Review", "(2 pending)")
	mustContain(t, dialog, "2 pending comments",
		`data-jump-thread="T2"`, "lex/new.go:4", "New thread here",
		`data-jump-thread="T1"`, "parser.go:2", "Reply", "A **draft** reply")
}

func TestCommentBoxesAreNotRestoredOrAutocompletedByTheBrowser(t *testing.T) {
	a := newApp(t)
	a.withThread()
	a.do(t, http.MethodGet, prPath, nil)
	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"}}
	for name, body := range map[string]string{
		"PR page":         a.do(t, http.MethodGet, prPath, nil).Body.String(),
		"file fragment":   a.do(t, http.MethodGet, fileURL(), nil).Body.String(),
		"new thread form": a.do(t, http.MethodGet, prPath+"/threads/new?"+q.Encode(), nil).Body.String(),
	} {
		for _, ta := range strings.Split(body, "<textarea")[1:] {
			tag := ta[:strings.Index(ta, ">")]
			if !strings.Contains(tag, `autocomplete="off"`) {
				t.Errorf("%s: textarea%s> can be restored or autocompleted", name, tag)
			}
		}
	}
}

// TestNoFormControlCanBeRestoredOrAutocompleted parses every page and
// fragment: each input, select and textarea must have autocomplete="off",
// itself or on its form, so reloads and browser history never fill in
// stale values that disagree with the page.
func TestNoFormControlCanBeRestoredOrAutocompleted(t *testing.T) {
	a := newApp(t)
	a.withThread()
	a.do(t, http.MethodGet, prPath, nil)
	a.repos.Blobs[c2] = map[string]string{"lib/util.go": "package lib\n"}
	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"}}
	for _, path := range []string{
		"/", prPath, fileURL(), prPath + "/threads/new?" + q.Encode(), prPath + "/blob?path=lib/util.go",
		"/settings", "/settings/servers/list", "/dashboard/lists",
	} {
		doc, err := html.Parse(strings.NewReader(a.do(t, http.MethodGet, path, nil).Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		var walk func(n *html.Node, formOff bool)
		walk = func(n *html.Node, formOff bool) {
			if n.Type == html.ElementNode {
				off := attr(n, "autocomplete") == "off"
				switch n.Data {
				case "form":
					formOff = off
				case "input", "select", "textarea":
					if !off && !formOff {
						t.Errorf("%s: <%s name=%q id=%q type=%q> can be restored or autocompleted",
							path, n.Data, attr(n, "name"), attr(n, "id"), attr(n, "type"))
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, formOff)
			}
		}
		walk(doc, false)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func TestSubmittedReviewsAppearInTheConversationInTimeOrder(t *testing.T) {
	a := newApp(t)
	at := func(m int) time.Time { return time.Date(2026, 11, 11, 9, m, 0, 0, time.UTC) }
	st := a.host.State(ref)
	st.Conversation = []domain.Comment{{ID: "IC1", Author: "sam", BodyHTML: "<p>Looks promising</p>", CreatedAt: at(10)}}
	st.Reviews = []domain.SubmittedReview{
		{ID: "R1", Author: "bo", Verdict: domain.ReviewChangesRequested, BodyHTML: "<p>Needs tests</p>", SubmittedAt: at(5)},
		{ID: "R2", Author: "ana", Verdict: domain.ReviewApproved, BodyHTML: "<p>Ship it</p>", SubmittedAt: at(20)},
		{ID: "R3", Author: "cy", Verdict: domain.ReviewCommented, SubmittedAt: at(25)},
	}

	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page, `Conversation (<span id="conversation-count">4</span>)`)
	conv := page[strings.Index(page, `id="conversation"`):]
	conv = conv[:strings.Index(conv, `id="conversation-new"`)]
	order := []string{"requested changes", "Looks promising", "approved these changes", "reviewed"}
	last := -1
	for _, want := range order {
		i := strings.Index(conv, want)
		if i < 0 || i < last {
			t.Fatalf("%q missing or out of order in:\n%s", want, conv)
		}
		last = i
	}
	mustContain(t, conv, "review-changes_requested", "<p>Needs tests</p>", "review-approved", "<p>Ship it</p>", "bo", "ana")
}

func TestASubmittedReviewShowsUpAfterSubmitting(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	a.do(t, http.MethodPost, prPath+"/review", url.Values{"head": {string(c2)}, "event": {"REQUEST_CHANGES"}, "body": {"Please add tests"}})

	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page, "requested changes", "Please add tests", `<span id="conversation-count">1</span>`)

	// Posting a comment keeps the review and counts both.
	rec := a.do(t, http.MethodPost, prPath+"/comments", url.Values{"body": {"Done"}})
	mustContain(t, rec.Body.String(), "requested changes", "Done", `<span id="conversation-count" hx-swap-oob="true">2</span>`)
}

func TestTheDescriptionShowsAboveTheConversationWithItsEdits(t *testing.T) {
	a := newApp(t)
	at := func(m int) time.Time { return time.Date(2026, 11, 11, 9, m, 0, 0, time.UTC) }
	st := a.host.State(ref)
	st.Conversation = []domain.Comment{{ID: "IC1", Author: "sam", BodyHTML: "<p>Looks promising</p>", CreatedAt: at(10)}}
	st.Description = domain.Description{
		Author: "ana", BodyHTML: "<p>Adds caching</p>", CreatedAt: at(0),
		Original: "Adds a cache\nKeep\n", HasOriginal: true,
		Edits: []domain.DescriptionEdit{{Editor: "bo", EditedAt: at(20), Body: "Adds caching\nKeep\n"}},
	}

	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	desc, conv := strings.Index(page, `id="description"`), strings.Index(page, `id="conversation"`)
	if desc < 0 || conv < desc {
		t.Fatalf("description at %d, conversation at %d", desc, conv)
	}
	mustContain(t, page[desc:conv], "ana", "<p>Adds caching</p>", "edited by bo")
	// The edit comes after the comment, showing the lines it changed.
	timeline := page[conv:]
	comment, edit := strings.Index(timeline, "Looks promising"), strings.Index(timeline, "edited the description")
	if comment < 0 || edit < comment {
		t.Fatalf("comment at %d, edit at %d in:\n%s", comment, edit, timeline)
	}
	mustContain(t, timeline, `<span class="del">− Adds a cache</span>`, `<span class="add">+ Adds caching</span>`, `<span class="ctx">  Keep</span>`)
	// Posting a comment refreshes the description too.
	rec := a.do(t, http.MethodPost, prPath+"/comments", url.Values{"body": {"Done"}})
	mustContain(t, rec.Body.String(), `id="description" class="description" hx-swap-oob="true"`, `<span id="conversation-count" hx-swap-oob="true">2</span>`)
}
