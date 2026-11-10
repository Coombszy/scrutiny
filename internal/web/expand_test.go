package web_test

import (
	"fmt"
	"net/http"
	"net/url"
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

func TestGapControlsSitOnTheHunkHeader(t *testing.T) {
	a := newApp(t)
	a.withLongParser()
	a.do(t, http.MethodGet, prPath, nil)
	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()

	// The middle gap's controls are on the second hunk's header row.
	row := body[strings.Index(body, "@@ -99,3 +100,3 @@"):]
	row = row[:strings.Index(row, "</tr>")]
	mustContain(t, row, `class="expand-actions"`, "↓ 64 lines", "88 hidden lines", "↑ 64 lines")
	if n := strings.Count(body, `class="expander"`); n != 1 {
		t.Errorf("%d standalone expander rows, want 1 (the end of the file)", n)
	}

	q := url.Values{"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "old": {"parser.go"}, "status": {"modified"},
		"start": {"12"}, "end": {"99"}, "offset": {"-1"}, "header": {"@@ -99,3 +100,3 @@"}}

	// Down: the shown lines, then the header (with the rest's controls) at the new break.
	q.Set("dir", "down")
	down := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil).Body.String()
	if !(strings.Index(down, `data-line="75"`) < strings.Index(down, "@@ -99,3 +100,3 @@")) {
		t.Error("expanding down should put the header after the shown lines")
	}
	mustContain(t, down, "Show 24 lines")

	// Up: the header (with the rest's controls), then the shown lines.
	q.Set("dir", "up")
	up := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil).Body.String()
	if !(strings.Index(up, "@@ -99,3 +100,3 @@") < strings.Index(up, `data-line="36"`)) {
		t.Error("expanding up should put the header before the shown lines")
	}

	// All: no break is left, so no header.
	q.Set("dir", "all")
	if all := a.do(t, http.MethodGet, prPath+"/context?"+q.Encode(), nil).Body.String(); strings.Contains(all, "@@") {
		t.Error("a fully opened gap kept its hunk header")
	}
}
