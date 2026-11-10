package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

var errBadRequest = errors.New("bad request")

var (
	shaPattern = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	// namePattern is GitHub's charset for owner and repository names.
	namePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// gitHubPR builds a github.com PRRef, rejecting names that are not valid on
// GitHub (including "." and "..", which would escape the clone directory).
func gitHubPR(owner, repo, number string) (domain.PRRef, bool) {
	n, err := strconv.Atoi(number)
	if err != nil || n < 1 {
		return domain.PRRef{}, false
	}
	for _, name := range []string{owner, repo} {
		if !namePattern.MatchString(name) || strings.Trim(name, ".") == "" {
			return domain.PRRef{}, false
		}
	}
	return domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: owner, Name: repo}, Number: n}, true
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, cached, err := s.review.CachedDashboard(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPage(w, r, http.StatusOK, views.Page{KeyHelp: true}, views.Dashboard(d, cached))
}

// dashboardLists fetches the Dashboard's GitHub lists for the page to swap in.
func (s *Server) dashboardLists(w http.ResponseWriter, r *http.Request) {
	d, err := s.review.Dashboard(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.DashboardLists(d))
}

// open redirects a pasted GitHub PR URL to the same PR in Scrutiny.
func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	u, err := url.Parse(strings.TrimSpace(r.URL.Query().Get("url")))
	var parts []string
	if err == nil && u.Host == "github.com" {
		parts = strings.Split(strings.Trim(u.Path, "/"), "/")
	}
	if len(parts) < 4 || parts[2] != "pull" {
		s.fail(w, r, fmt.Errorf("%w: not a github.com pull request URL", errBadRequest))
		return
	}
	ref, ok := gitHubPR(parts[0], parts[1], parts[3])
	if !ok {
		s.fail(w, r, fmt.Errorf("%w: not a github.com pull request URL", errBadRequest))
		return
	}
	http.Redirect(w, r, ref.Path(), http.StatusSeeOther)
}

func (s *Server) pullRequest(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	req := review.ComparisonRequest{First: domain.SHA(q.Get("first")), Last: domain.SHA(q.Get("last"))}
	page, err := s.review.PRPage(r.Context(), ref, req)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cfg, err := s.settings.Load()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPage(w, r, http.StatusOK, views.Page{Indexing: s.review.Indexing(ref), KeyHelp: true, CodeHelp: true}, views.PR(page, cfg.ViewMode))
}

// file renders one file's diff as an htmx fragment.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	base, head := q.Get("base"), q.Get("head")
	change := domain.FileChange{Path: q.Get("path"), OldPath: q.Get("old"), Status: domain.ChangeStatus(q.Get("status"))}
	if !shaPattern.MatchString(base) || !shaPattern.MatchString(head) || change.Path == "" || change.OldPath == "" {
		s.fail(w, r, fmt.Errorf("%w: base, head, path and old are required", errBadRequest))
		return
	}
	d, err := s.review.FileDiff(r.Context(), ref, domain.SHA(base), domain.SHA(head), change, q.Get("full") == "1")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	dc, err := s.diffContext(r, ref, domain.SHA(base), domain.SHA(head), change)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.Diff(dc, d))
}

// diffContext is what rendering rows of a file's diff needs.
func (s *Server) diffContext(r *http.Request, ref domain.PRRef, base, head domain.SHA, change domain.FileChange) (views.DiffContext, error) {
	anchors, err := s.review.Anchors(r.Context(), ref)
	if err != nil {
		return views.DiffContext{}, err
	}
	cfg, err := s.settings.Load()
	if err != nil {
		return views.DiffContext{}, err
	}
	return views.DiffContext{
		PR: ref, Base: base, Head: head, Mode: cfg.ViewMode, File: change, ExpandLines: cfg.Diff.ExpandLines,
		CanComment:     anchors.Pending == nil || anchors.Pending.Commit == head,
		CanCommentLeft: base == anchors.MergeBase,
	}, nil
}

// expandGap reveals unchanged lines from a gap in a file's diff.
func (s *Server) expandGap(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	base, head := q.Get("base"), q.Get("head")
	change := domain.FileChange{Path: q.Get("path"), OldPath: q.Get("old"), Status: domain.ChangeStatus(q.Get("status"))}
	start, err1 := strconv.Atoi(q.Get("start"))
	end, err2 := strconv.Atoi(q.Get("end"))
	offset, err3 := strconv.Atoi(q.Get("offset"))
	dir := review.Direction(q.Get("dir"))
	hunk := q.Get("hunk")
	if !shaPattern.MatchString(base) || !shaPattern.MatchString(head) || change.Path == "" ||
		err1 != nil || err2 != nil || err3 != nil || (dir != review.Up && dir != review.Down && dir != review.All) ||
		(hunk != "" && !views.HunkIDPattern.MatchString(hunk)) {
		s.fail(w, r, fmt.Errorf("%w: base, head, path, start, end, offset and dir are required", errBadRequest))
		return
	}
	gap := review.Gap{Start: start, End: end, Offset: offset, First: q.Get("first") == "true", Last: q.Get("last") == "true"}
	dc, err := s.diffContext(r, ref, domain.SHA(base), domain.SHA(head), change)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	shown, rest, restBefore := gap.Reveal(dir, dc.ExpandLines)
	lines, err := s.review.ContextLines(r.Context(), ref, domain.SHA(head), change.Path, shown)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ExpandedRows(dc, lines, rest, restBefore, hunk))
}

// setViewMode saves the View Mode to the config file and reloads the page.
func (s *Server) setViewMode(w http.ResponseWriter, r *http.Request) {
	mode := settings.ViewMode(r.PostFormValue("mode"))
	s.updateSettings(w, r, func(cfg *settings.Settings) { cfg.ViewMode = mode }, true)
}

func prRef(r *http.Request) (domain.PRRef, bool) {
	return gitHubPR(r.PathValue("owner"), r.PathValue("repo"), r.PathValue("number"))
}
