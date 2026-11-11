package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
)

func (a *app) withOwnComment() {
	a.withThread()
	st := a.host.State(ref)
	st.Pending = &domain.PendingReview{ID: "PRR1", Commit: c2, Comments: 1}
	st.Threads[0].Comments = append(st.Threads[0].Comments, domain.Comment{
		ID: "C2", Author: "me", Body: "Because", BodyHTML: "<p>Because</p>", Pending: true, ViewerCanUpdate: true, ViewerCanDelete: true,
	})
	st.Conversation = []domain.Comment{{ID: "IC1", Author: "me", Body: "Thanks", BodyHTML: "<p>Thanks</p>", ViewerCanUpdate: true, ViewerCanDelete: true}}
}

func TestOwnCommentsOfferEditAndDelete(t *testing.T) {
	a := newApp(t)
	a.withOwnComment()
	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page,
		`hx-post="/github.com/o/r/pull/7/comments/IC1/edit"`, `hx-post="/github.com/o/r/pull/7/comments/IC1/delete"`,
		`hx-target="#conversation"`, ">Thanks</textarea>",
	)
	// Threads arrive with their file's diff.
	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()
	mustContain(t, body,
		`hx-post="/github.com/o/r/pull/7/comments/C2/edit"`, `hx-post="/github.com/o/r/pull/7/comments/C2/delete"`,
		`hx-target="closest .thread"`, "data-edit-comment", "hx-confirm=", `data-confirm-label="Delete"`, ">Because</textarea>",
	)
	if strings.Contains(body, "comments/C1/") {
		t.Error("offers to change someone else's comment")
	}
}

func TestEditingACommentReturnsItsThread(t *testing.T) {
	a := newApp(t)
	a.withOwnComment()
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodPost, prPath+"/comments/C2/edit", url.Values{"body": {"Because it is new"}})
	mustContain(t, rec.Body.String(), `id="thread-T1"`, "Because it is new", `id="pending-count" hx-swap-oob="true"`)

	rec = a.do(t, http.MethodPost, prPath+"/comments/IC1/edit", url.Values{"body": {"Thanks all"}})
	mustContain(t, rec.Body.String(), `id="conversation"`, "Thanks all", `<span id="conversation-count" hx-swap-oob="true">1</span>`)
}

func TestDeletingCommentsUpdatesThePage(t *testing.T) {
	a := newApp(t)
	a.withOwnComment()
	a.do(t, http.MethodGet, prPath, nil)

	rec := a.do(t, http.MethodPost, prPath+"/comments/C2/delete", formValues())
	body := rec.Body.String()
	mustContain(t, body, `id="thread-T1"`, `id="pending-count" hx-swap-oob="true"`)
	if strings.Contains(body, "comment-C2") || strings.Contains(body, "(1 pending)") {
		t.Errorf("the deleted pending reply is still shown: %s", body)
	}

	rec = a.do(t, http.MethodPost, prPath+"/comments/IC1/delete", formValues())
	mustContain(t, rec.Body.String(), `id="conversation"`, `<span id="conversation-count" hx-swap-oob="true">0</span>`)
}

func TestChangingSomeoneElsesCommentIsRefused(t *testing.T) {
	a := newApp(t)
	a.withOwnComment()
	a.do(t, http.MethodGet, prPath, nil)
	if rec := a.do(t, http.MethodPost, prPath+"/comments/C1/delete", formValues()); rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rec.Code)
	}
}

func TestCommentBoxesOfferPreviewAndFormatting(t *testing.T) {
	a := newApp(t)
	a.withThread()
	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page, `data-preview="/github.com/o/r/pull/7/markdown"`, `data-md-tab="preview"`, `data-md="bold"`)
	if strings.Contains(page, `data-md="suggestion"`) {
		t.Error("a Conversation Comment offers a suggested change")
	}
	// A thread on the head side can suggest a change to its line.
	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()
	mustContain(t, body, `data-md="suggestion" data-start="2" data-end="2"`)
}

func TestPreviewRendersMarkdown(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, prPath+"/markdown", url.Values{"body": {"<b>hi</b>"}})
	if rec.Code != http.StatusOK || rec.Body.String() != "<p>&lt;b&gt;hi&lt;/b&gt;</p>" {
		t.Errorf("preview = %d %q", rec.Code, rec.Body.String())
	}
	rec = a.do(t, http.MethodPost, prPath+"/markdown", url.Values{"body": {"  "}})
	mustContain(t, rec.Body.String(), "Nothing to preview")
}
