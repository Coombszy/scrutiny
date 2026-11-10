package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/live"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/web"
)

var (
	repo = domain.Repo{Host: "github.com", Owner: "o", Name: "r"}
	ref  = domain.PRRef{Repo: repo, Number: 7}
)

const (
	tip domain.SHA = "1111111111111111111111111111111111111111"
	mb  domain.SHA = "2222222222222222222222222222222222222222"
	c1  domain.SHA = "3333333333333333333333333333333333333333"
	c2  domain.SHA = "4444444444444444444444444444444444444444"
)

type app struct {
	nav      *navigator.Fake
	hub      *live.Hub
	host     *codehost.Fake
	repos    *repostore.Fake
	settings *settings.Memory
	handler  http.Handler
}

func newApp(t *testing.T) *app {
	t.Helper()
	a := &app{nav: navigator.NewFake(), hub: live.NewHub(), host: codehost.NewFake(), repos: repostore.NewFake(), settings: settings.NewMemory(settings.Defaults())}
	a.host.PRs[ref] = domain.PullRequest{Ref: ref, Title: "Teach the parser <generics>", Author: "octo", BaseBranch: "main", HeadBranch: "feat", HeadSHA: c2, State: domain.Open}
	a.host.CommitsOf[ref] = []domain.Commit{{SHA: c1, Headline: "first step"}, {SHA: c2, Headline: "second step"}}
	a.repos.Remotes[ref] = repostore.FakeRemote{BaseTip: tip, Commits: []domain.SHA{c1, c2}}
	a.repos.MergeBases[repostore.Pair{From: tip, To: c2}] = mb
	a.repos.ChangeSets[repostore.Pair{From: mb, To: c2}] = []domain.FileChange{
		{Path: "parser.go", OldPath: "parser.go", Status: domain.Modified, Additions: 2, Deletions: 1},
		{Path: "go.sum", OldPath: "go.sum", Status: domain.Modified, Additions: 4},
		{Path: "logo.png", OldPath: "logo.png", Status: domain.Modified, Binary: true},
		{Path: "lex/new.go", OldPath: "lex/old.go", Status: domain.Renamed},
	}
	a.repos.Diffs[repostore.Pair{From: mb, To: c2}] = map[string]domain.FileDiff{"parser.go": {Hunks: []domain.Hunk{{
		OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3,
		Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 1, NewNo: 1, Text: "package parser"},
			{Kind: domain.Delete, OldNo: 2, Text: "var old = 1"},
			{Kind: domain.Add, NewNo: 2, Text: "var renewed = 1"},
			{Kind: domain.Add, NewNo: 3, Text: "var extra = 2"},
		},
	}}}}
	svc := review.NewService(review.Deps{
		CodeHost: a.host, Repos: a.repos, Highlighter: highlight.PlainHighlighter{},
		Cache: cache.NewMemory(), Settings: a.settings, Navigator: a.nav,
	})
	a.handler = web.NewServer(token, svc, a.settings, a.hub).Handler()
	return a
}

func (a *app) do(t *testing.T, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(method, target, body)
	req.Host = "127.0.0.1:7878"
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("X-CSRF-Token", token)
	}
	rec := httptest.NewRecorder()
	a.handler.ServeHTTP(rec, req)
	return rec
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("response does not contain %q", w)
		}
	}
}

func TestPRPageListsFilesForLazyLoading(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodGet, "/github.com/o/r/pull/7", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	mustContain(t, body,
		"Teach the parser &lt;generics&gt;",
		`hx-trigger="intersect once"`,
		"/github.com/o/r/pull/7/file?",
		"Load diff",   // go.sum is generated
		"Binary file", // logo.png
		"lex/old.go → lex/new.go",
		"first step", "second step", // the Comparison picker
	)
}

func TestPRPageUnknownPRAndHostAre404(t *testing.T) {
	a := newApp(t)
	for _, path := range []string{"/github.com/o/r/pull/8", "/gitlab.com/o/r/pull/7", "/github.com/o/r/pull/x"} {
		if rec := a.do(t, http.MethodGet, path, nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
	}
}

func TestPRPageRejectsAnInvalidCommitRange(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodGet, "/github.com/o/r/pull/7?first="+string(c2)+"&last="+string(c1), nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

func fileURL() string {
	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "old": {"parser.go"}, "status": {"modified"}}
	return "/github.com/o/r/pull/7/file?" + q.Encode()
}

func TestFileFragmentRendersTheConfiguredViewMode(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodGet, fileURL(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	mustContain(t, rec.Body.String(), `class="diff split"`, "var renewed = 1", "@@ -1,2 +1,3 @@")

	cfg := settings.Defaults()
	cfg.ViewMode = settings.Unified
	a.settings.Save(cfg)
	rec = a.do(t, http.MethodGet, fileURL(), nil)
	mustContain(t, rec.Body.String(), `class="diff unified"`, "var old = 1")
}

func TestToggleViewModeWritesSettings(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, "/settings/view-mode", url.Values{"mode": {"unified"}})
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("status %d, HX-Refresh %q", rec.Code, rec.Header().Get("HX-Refresh"))
	}
	if cfg, _ := a.settings.Load(); cfg.ViewMode != settings.Unified {
		t.Errorf("ViewMode = %q, want unified", cfg.ViewMode)
	}
	if rec := a.do(t, http.MethodPost, "/settings/view-mode", url.Values{"mode": {"diagonal"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid mode: status %d, want 400", rec.Code)
	}
}

func TestDashboardListsPRs(t *testing.T) {
	a := newApp(t)
	a.host.Dash = codehost.Dashboard{
		ReviewRequested: []domain.PRSummary{{Ref: domain.PRRef{Repo: repo, Number: 9}, Title: "Needs eyes", Author: "pat"}},
		Authored:        []domain.PRSummary{{Ref: ref, Title: "My work"}},
	}
	a.do(t, http.MethodGet, "/github.com/o/r/pull/7", nil) // a visit

	rec := a.do(t, http.MethodGet, "/", nil)
	mustContain(t, rec.Body.String(),
		"Review requested", "Needs eyes", `href="/github.com/o/r/pull/9"`,
		"Authored", "My work",
		"Recently opened", "Teach the parser",
	)
}

func TestOpenRedirectsGitHubURLs(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodGet, "/open?url="+url.QueryEscape("https://github.com/o/r/pull/7/files"), nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/github.com/o/r/pull/7" {
		t.Errorf("status %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = a.do(t, http.MethodGet, "/open?url="+url.QueryEscape("https://example.com/x"), nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("non-PR URL: status %d, want 400", rec.Code)
	}
}

func TestPRPathsWithTraversalOrInvalidNamesAre404(t *testing.T) {
	a := newApp(t)
	// Were the names not validated, this PR would be found and served.
	evil := domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: "../../x", Name: "y"}, Number: 7}
	a.host.PRs[evil] = a.host.PRs[ref]
	for _, path := range []string{
		"/github.com/..%2F..%2Fx/y/pull/7",
		"/github.com/o/..%2Fr/pull/7",
		"/github.com/../r/pull/7/file",
		"/github.com/o/.../pull/7",
		"/github.com/o%20o/r/pull/7",
	} {
		if rec := a.do(t, http.MethodGet, path, nil); rec.Code != http.StatusNotFound && rec.Code != http.StatusTemporaryRedirect {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
	}
}
