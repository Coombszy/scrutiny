package web_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Coombszy/scrutiny/internal/navigator"
)

func TestDefinitionReturnsTargetsWithTheirURLs(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	a.nav.Definitions[navigator.Query{Path: "parser.go", Pos: navigator.Position{Line: 2, Col: 6}}] = []navigator.Location{
		{Path: "parser.go", Line: 1, Col: 8},
		{Path: "lib/util.go", Line: 40, Col: 5},
	}

	rec := a.do(t, http.MethodGet, prPath+"/definition?path=parser.go&line=2&col=6&head="+string(c2), nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, type %q: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body)
	}
	var got []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		InPR bool   `json:"inPR"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].InPR || got[1].InPR || got[1].URL != prPath+"/blob?path=lib%2Futil.go#L40" {
		t.Errorf("targets = %+v", got)
	}
}

func TestNavigatingWithoutALanguageServerSaysSo(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodGet, prPath+"/definition?path=notes.txt&line=1&col=0&head="+string(c2), nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	mustContain(t, rec.Body.String(), "no language server")
}

func TestReferencesPanelListsUsagesWithText(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	a.repos.Blobs[c2] = map[string]string{"parser.go": "package parser\nvar renewed = 1\n", "use.go": "package parser\n\nvar x = renewed\n"}
	a.nav.Refs[navigator.Query{Path: "parser.go", Pos: navigator.Position{Line: 2, Col: 4}}] = []navigator.Location{
		{Path: "parser.go", Line: 2, Col: 4}, {Path: "use.go", Line: 3, Col: 8},
	}

	body := a.do(t, http.MethodGet, prPath+"/references?path=parser.go&line=2&col=4&head="+string(c2), nil).Body.String()
	mustContain(t, body, "2 usages", "use.go", "var x = renewed", `data-nav-path="use.go"`, `data-nav-line="3"`, `href="/github.com/o/r/pull/7/blob?path=use.go#L3"`)
}

func TestSymbolSearchListsMarkedResults(t *testing.T) {
	a := newApp(t)
	a.repos.Blobs[c2] = map[string]string{"parser.go": "", "lib/parse_test.go": ""}
	a.nav.AllSymbols = []navigator.Symbol{{Name: "ParseFile", Kind: "function", Location: navigator.Location{Path: "lib/parse_test.go", Line: 9}}}
	a.do(t, http.MethodGet, prPath, nil)

	body := a.do(t, http.MethodGet, prPath+"/symbols?q=pars", nil).Body.String()
	mustContain(t, body, "parser.go", "ParseFile", "function", `class="result in-pr`, `data-nav-path="lib/parse_test.go"`, `data-nav-line="9"`)
}

func TestFileViewShowsTheWholeFileAtHead(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	a.repos.Blobs[c2] = map[string]string{"lib/util.go": "package lib\n\nfunc Util() {}\n"}

	rec := a.do(t, http.MethodGet, prPath+"/blob?path=lib/util.go", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	mustContain(t, rec.Body.String(), "lib/util.go", `id="L3"`, `data-line="3"`, "func Util() {}", `data-path="lib/util.go"`, `id="palette"`)
}

func TestDiffCellsCarryNavigationAnchors(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	body := a.do(t, http.MethodGet, fileURL(), nil).Body.String()
	mustContain(t, body, `data-path="parser.go"`, `data-line="2"`, `data-head="`+string(c2)+`"`)
	page := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, page, `id="indexing"`, `id="palette"`, `data-path="parser.go"`)
}

func TestNavigatingFromAStaleViewIsAConflict(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	rec := a.do(t, http.MethodGet, prPath+"/definition?path=parser.go&line=2&col=6&head="+string(c1), nil)
	if rec.Code != http.StatusConflict {
		t.Errorf("status %d, want 409", rec.Code)
	}
	if rec := a.do(t, http.MethodGet, prPath+"/definition?path=../x.go&line=2&col=6&head="+string(c2), nil); rec.Code != http.StatusBadRequest {
		t.Errorf("bad path: status %d, want 400", rec.Code)
	}
}
