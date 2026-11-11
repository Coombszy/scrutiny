package web_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

func TestPRSearchIsOnEveryPage(t *testing.T) {
	a := newApp(t)
	for _, path := range []string{"/", prPath, "/settings"} {
		mustContain(t, a.do(t, http.MethodGet, path, nil).Body.String(), `id="pr-search"`, `data-src="/prs"`, `pr_search`, `ctrl+p`)
	}
}

func TestPRSearchListsTheDashboardThenRecentPRsOnce(t *testing.T) {
	a := newApp(t)
	nine := domain.PRRef{Repo: repo, Number: 9}
	a.host.Dash = codehost.Dashboard{
		ReviewRequested: []domain.PRSummary{{Ref: nine, Title: "Needs eyes", Author: "pat", Draft: true}},
		Authored:        []domain.PRSummary{{Ref: ref, Title: "My work", Author: "octo"}},
	}
	a.do(t, http.MethodGet, prPath, nil) // a visit

	type pr struct {
		Title, Ref, Author, Href string
		Draft                    bool
	}
	get := func(path string) (prs []pr, stale bool) {
		t.Helper()
		rec := a.do(t, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
			t.Fatalf("%s: status %d, Content-Type %q: %s", path, rec.Code, rec.Header().Get("Content-Type"), rec.Body)
		}
		var out struct {
			PRs   []pr
			Stale bool
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.PRs, out.Stale
	}
	want := []pr{
		{Title: "Needs eyes", Ref: "github.com/o/r#9", Author: "pat", Href: "/github.com/o/r/pull/9", Draft: true},
		{Title: "My work", Ref: "github.com/o/r#7", Author: "octo", Href: "/github.com/o/r/pull/7"},
	}

	// Never fetched from GitHub: only the recent visit, and stale.
	if got, stale := get("/prs"); len(got) != 1 || got[0].Title != "Teach the parser <generics>" || got[0].Href != "/github.com/o/r/pull/7" || !stale {
		t.Errorf("before fetching: %+v, stale %v", got, stale)
	}

	// refresh fetches the lists from GitHub, as the Dashboard does.
	if got, stale := get("/prs?refresh=1"); len(got) != 2 || got[0] != want[0] || got[1] != want[1] || stale {
		t.Errorf("refreshed: %+v, stale %v; want %+v", got, stale, want)
	}

	// Within ten minutes the cache answers and is fresh.
	a.host.Dash = codehost.Dashboard{}
	if got, stale := get("/prs"); len(got) != 2 || got[0] != want[0] || stale {
		t.Errorf("cached: %+v, stale %v", got, stale)
	}
}
