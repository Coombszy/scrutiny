package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/settings"
)

// withLongParser makes parser.go 200 lines at head, changed at line 11 and
// at line 101: gaps 1-9, 12-99 and 103-200.
func (a *app) withLongParser() {
	var head []string
	for i := 1; i <= 200; i++ {
		head = append(head, fmt.Sprintf("line %d", i))
	}
	a.repos.Blobs[c2] = map[string]string{"parser.go": strings.Join(head, "\n") + "\n"}
	change := domain.FileChange{Path: "parser.go", OldPath: "parser.go", Status: domain.Modified, Additions: 2, Deletions: 1}
	a.repos.Diffs[repostore.Pair{From: mb, To: c2}]["parser.go"] = domain.FileDiff{FileChange: change, Hunks: []domain.Hunk{
		{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 2, Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 10, NewNo: 10}, {Kind: domain.Add, NewNo: 11},
		}},
		{OldStart: 99, OldLines: 3, NewStart: 100, NewLines: 3, Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 99, NewNo: 100}, {Kind: domain.Delete, OldNo: 100},
			{Kind: domain.Add, NewNo: 101}, {Kind: domain.Context, OldNo: 101, NewNo: 102},
		}},
	}}
}

func TestGapsBetweenChangesCanBeExpanded(t *testing.T) {
	a := newApp(t)
	a.withLongParser()
	a.do(t, http.MethodGet, prPath, nil)

	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()
	mustContain(t, body, `class="expander"`, "Show 9 lines", "88 hidden lines", "↓ 64 lines", "↑ 64 lines", "98 hidden lines",
		prPath+"/context?")
	if strings.Contains(body, `data-line="50"`) {
		t.Error("an unchanged line is shown before expanding")
	}

	// Expanding the middle gap downwards shows 64 lines and leaves the rest.
	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "old": {"parser.go"}, "status": {"modified"},
		"start": {"12"}, "end": {"99"}, "offset": {"-1"}, "dir": {"down"}}
	rows := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil).Body.String()
	mustContain(t, rows, `data-line="12"`, "line 12", `data-line="75"`, "Show 24 lines")
	if strings.Contains(rows, `data-line="76"`) {
		t.Error("revealed more than 64 lines")
	}
	if !strings.Contains(rows, `<td class="ln ctx">11</td>`) {
		t.Error("revealed lines lack their base line numbers")
	}
}

func TestTheExpandStepFollowsTheSetting(t *testing.T) {
	a := newApp(t)
	a.withLongParser()
	cfg := settings.Defaults()
	cfg.Diff.ExpandLines = 10
	a.settings.Save(cfg)
	a.do(t, http.MethodGet, prPath, nil)
	mustContain(t, a.do(t, http.MethodGet, fileURL(), nil).Body.String(), "↓ 10 lines", "↑ 10 lines", "Show 9 lines")
}

func TestFullFileShowsEveryLine(t *testing.T) {
	a := newApp(t)
	a.withLongParser()
	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page, `data-full-toggle`, `data-file-url="`+prPath+`/file?`)

	body := a.do(t, http.MethodGet, fileURL()+"&full=1", nil).Body.String()
	mustContain(t, body, `data-line="1"`, `data-line="50"`, `data-line="200"`)
	if strings.Contains(body, `class="expander"`) {
		t.Error("full file still has gaps")
	}
}

func TestOpeningAGapFullyRemovesTheHunkHeaderBelowIt(t *testing.T) {
	a := newApp(t)
	a.withLongParser()
	a.do(t, http.MethodGet, prPath, nil)
	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()

	// The second hunk's header, and the gap above it that names it.
	header := regexp.MustCompile(`<tr class="hunk" id="(hunk-[0-9a-f]+-1)">`).FindStringSubmatch(body)
	if header == nil {
		t.Fatalf("no id on the second hunk header:\n%s", body)
	}
	mustContain(t, body, "hunk="+header[1])

	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "old": {"parser.go"}, "status": {"modified"},
		"start": {"12"}, "end": {"99"}, "offset": {"-1"}, "hunk": {header[1]}}
	q.Set("dir", "down")
	partial := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil).Body.String()
	if strings.Contains(partial, `hx-swap-oob="delete"`) {
		t.Error("removed the header while lines are still hidden")
	}
	mustContain(t, partial, "hunk="+header[1]) // the remaining gap still names it
	q.Set("dir", "all")
	whole := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil).Body.String()
	mustContain(t, whole, `<tr id="`+header[1]+`" hx-swap-oob="delete"></tr>`)

	q.Set("hunk", "nope; drop table")
	if rec := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad hunk id: status %d, want 400", rec.Code)
	}
}
