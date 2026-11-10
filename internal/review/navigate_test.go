package review_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

var goWorkspace = navigator.Workspace{Root: "/worktrees/github.com/o/r#7", Language: "go"}

func TestDefinitionRunsInAWorktreeAtTheHead(t *testing.T) {
	f := newFixture(t)
	f.openPage(t) // a.go changed
	f.nav.Definitions[navigator.Query{Path: "a.go", Pos: navigator.Position{Line: 2, Col: 4}}] = []navigator.Location{
		{Path: "a.go", Line: 1, Col: 8},
		{Path: "lib/b.go", Line: 10},
		{Path: "/usr/lib/go/src/fmt/print.go", Line: 270, OutsideProject: true},
	}

	got, err := f.svc.Definition(context.Background(), ref, c3, "a.go", navigator.Position{Line: 2, Col: 4})
	if err != nil {
		t.Fatal(err)
	}
	if f.repos.Worktrees[ref] != c3 || f.nav.Calls[0] != goWorkspace {
		t.Errorf("worktree at %s, workspace %+v", f.repos.Worktrees[ref].Short(), f.nav.Calls[0])
	}
	if len(got) != 3 || !got[0].InPR || got[1].InPR || !got[2].OutsideProject {
		t.Errorf("Definition = %+v, want a.go marked as in the PR", got)
	}
}

func TestNavigatingAfterTheHeadMovesResetsTheServer(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	ctx := context.Background()
	if _, err := f.svc.Definition(ctx, ref, c3, "a.go", navigator.Position{Line: 1}); err != nil {
		t.Fatal(err)
	}

	const c4 domain.SHA = "c400000000000000000000000000000000000000"
	f.host.PRs[ref] = domain.PullRequest{Ref: ref, BaseBranch: "main", HeadSHA: c4}
	f.repos.Remotes[ref] = repostore.FakeRemote{BaseTip: tip, Commits: []domain.SHA{c1, c2, c3, c4}}
	f.repos.MergeBases[repostore.Pair{From: tip, To: c4}] = mb
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c4}] = nil
	if _, err := f.svc.Poll(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Definition(ctx, ref, c4, "a.go", navigator.Position{Line: 1}); err != nil {
		t.Fatal(err)
	}
	if f.repos.Worktrees[ref] != c4 || len(f.nav.Resets) != 1 {
		t.Errorf("worktree at %s, resets %v; want the worktree moved and the server reset", f.repos.Worktrees[ref].Short(), f.nav.Resets)
	}
}

func TestUnsupportedLanguagesHaveNoServer(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	_, err := f.svc.Definition(context.Background(), ref, c3, "notes.txt", navigator.Position{Line: 1})
	if !errors.Is(err, navigator.ErrNoServer) {
		t.Errorf("err = %v, want ErrNoServer", err)
	}
}

func TestReferencesCarryTheirLineText(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	f.repos.Blobs[c3] = map[string]string{"a.go": "package a\nvar x = 2\n", "b.go": "package a\n\nvar y = x\n"}
	f.nav.Refs[navigator.Query{Path: "a.go", Pos: navigator.Position{Line: 2, Col: 4}}] = []navigator.Location{
		{Path: "a.go", Line: 2, Col: 4}, {Path: "b.go", Line: 3, Col: 8},
	}

	got, err := f.svc.References(context.Background(), ref, c3, "a.go", navigator.Position{Line: 2, Col: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Text != "var x = 2" || got[1].Text != "var y = x" || !got[0].InPR || got[1].InPR {
		t.Errorf("References = %+v", got)
	}
}

func TestSymbolSearchWeightsThePRsFilesAndSymbolsFirst(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3)
	ch := f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}]
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = append(ch,
		domain.FileChange{Path: "parser/new.go", OldPath: "parser/new.go", Status: domain.Added},
		domain.FileChange{Path: "parser/old.go", OldPath: "parser/old.go", Status: domain.Deleted},
	)
	f.repos.Diffs[repostore.Pair{From: mb, To: c3}]["parser/new.go"] = domain.FileDiff{Hunks: []domain.Hunk{{NewStart: 1, NewLines: 3, Lines: []domain.Line{
		{Kind: domain.Add, NewNo: 1}, {Kind: domain.Add, NewNo: 2}, {Kind: domain.Add, NewNo: 3},
	}}}}
	f.repos.Blobs[c3] = map[string]string{"a.go": "", "parser/new.go": "", "vendor/parser_util.go": "", "parse.md": ""}
	f.nav.AllSymbols = []navigator.Symbol{
		{Name: "ParseAll", Kind: "function", Location: navigator.Location{Path: "vendor/parser_util.go", Line: 5}},
		{Name: "Parse", Kind: "function", Location: navigator.Location{Path: "parser/new.go", Line: 2}},
	}
	if _, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}

	got, err := f.svc.SymbolSearch(context.Background(), ref, "pars")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name   string
		marker review.Marker
	}
	var rows []row
	for _, r := range got {
		rows = append(rows, row{r.Name, r.Marker})
	}
	// PR items first; within each group the better match first.
	want := []row{
		{"Parse", review.MarkerAdded},
		{"parser/new.go", review.MarkerAdded},
		{"parser/old.go", review.MarkerRemoved},
		{"ParseAll", review.MarkerNone},
		{"parse.md", review.MarkerNone},
		{"vendor/parser_util.go", review.MarkerNone},
	}
	if len(rows) != len(want) {
		t.Fatalf("results = %+v\nwant %+v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v (all: %+v)", i, rows[i], want[i], rows)
		}
	}
}

func TestFileViewHighlightsTheFileAtHead(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	f.repos.Blobs[c3] = map[string]string{"lib/c.go": "package lib\n\nfunc C() {}\n"}

	got, err := f.svc.FileView(context.Background(), ref, "lib/c.go")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "lib/c.go" || got.Head != c3 || len(got.Lines) != 3 || got.InPR {
		t.Errorf("FileView = %+v", got)
	}
}

func TestClosingAPRRemovesItsWorktree(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	if _, err := f.svc.Definition(context.Background(), ref, c3, "a.go", navigator.Position{Line: 1}); err != nil {
		t.Fatal(err)
	}
	f.host.PRs[ref] = domain.PullRequest{Ref: ref, BaseBranch: "main", HeadSHA: c3, State: domain.Merged}

	if _, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.repos.Worktrees[ref]; ok || len(f.nav.Resets) != 1 {
		t.Errorf("worktree kept (%v) or server not reset (%v) after merge", ok, f.nav.Resets)
	}
}

func TestNavigatingFromAViewNotAtTheHeadIsRefused(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	// e.g. a Comparison ending at c2, or a page from before a push.
	_, err := f.svc.Definition(context.Background(), ref, c2, "a.go", navigator.Position{Line: 1})
	if !errors.Is(err, review.ErrNotAtHead) {
		t.Errorf("err = %v, want ErrNotAtHead", err)
	}
	if len(f.nav.Calls) != 0 {
		t.Error("asked the Language Server anyway")
	}
}

func TestNavigationPathsMustStayInTheRepository(t *testing.T) {
	f := newFixture(t)
	f.openPage(t)
	for _, p := range []string{"../../etc/passwd.go", "/etc/x.go", "a/../../b.go"} {
		if _, err := f.svc.Definition(context.Background(), ref, c3, p, navigator.Position{Line: 1}); !errors.Is(err, review.ErrBadPath) {
			t.Errorf("%s: err = %v, want ErrBadPath", p, err)
		}
	}
}

func TestSymbolSearchRanksAnExactMatchAboveAnUnchangedSymbolInAModifiedFile(t *testing.T) {
	f := newFixture(t)
	f.withDiff(mb, c3) // a.go is modified, adding line 2 only
	f.repos.Blobs[c3] = map[string]string{"a.go": ""}
	f.nav.AllSymbols = []navigator.Symbol{
		{Name: "Loader", Kind: "struct", Location: navigator.Location{Path: "lib/load.go", Line: 3}},
		{Name: "LoadOrder", Kind: "function", Location: navigator.Location{Path: "a.go", Line: 1}},
	}
	if _, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.SymbolSearch(context.Background(), ref, "loader")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "Loader" {
		t.Errorf("results = %+v, want the exact match first", got)
	}
}
