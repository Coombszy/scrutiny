package review_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

// lineTagger tags every line with the path and line number it was
// highlighted from, so tests can see which blob each diff line came from.
type lineTagger struct{}

func (lineTagger) Highlight(path string, src []byte) [][]highlight.Token {
	lines := highlight.Plain(src)
	for i := range lines {
		lines[i][0].Class = path + "@" + strconv.Itoa(i+1)
	}
	return lines
}

func TestFileDiffHighlightsEachSideFromItsOwnBlob(t *testing.T) {
	f := newFixture(t)
	f.svc.Highlighter = lineTagger{}
	change := domain.FileChange{Path: "new.go", OldPath: "old.go", Status: domain.Renamed}
	f.repos.Blobs[mb] = map[string]string{"old.go": "x\ny\n"}
	f.repos.Blobs[c3] = map[string]string{"new.go": "x\nY\nz\n"}
	f.repos.Diffs[repostore.Pair{From: mb, To: c3}] = map[string]domain.FileDiff{"new.go": {
		FileChange: change,
		Hunks: []domain.Hunk{{OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3, Section: "func f()", Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 1, NewNo: 1, Text: "x"},
			{Kind: domain.Delete, OldNo: 2, Text: "y"},
			{Kind: domain.Add, NewNo: 2, Text: "Y"},
			{Kind: domain.Add, NewNo: 3, Text: "z"},
		}}},
	}}

	got, err := f.svc.FileDiff(context.Background(), ref, mb, c3, change, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hunks) != 1 || got.Hunks[0].Header != "@@ -1,2 +1,3 @@ func f()" {
		t.Fatalf("hunks = %+v", got.Hunks)
	}
	want := []highlight.Token{
		{Text: "x", Class: "new.go@1"},
		{Text: "y", Class: "old.go@2"},
		{Text: "Y", Class: "new.go@2"},
		{Text: "z", Class: "new.go@3"},
	}
	for i, line := range got.Hunks[0].Lines {
		if len(line.Tokens) != 1 || line.Tokens[0] != want[i] {
			t.Errorf("line %d tokens = %+v, want %+v", i, line.Tokens, want[i])
		}
	}
}

func TestFileDiffFallsBackToPlainTextWithoutBlobs(t *testing.T) {
	f := newFixture(t)
	change := domain.FileChange{Path: "a.go", OldPath: "a.go", Status: domain.Added}
	f.repos.Diffs[repostore.Pair{From: mb, To: c3}] = map[string]domain.FileDiff{"a.go": {
		FileChange: change,
		Hunks: []domain.Hunk{{NewStart: 1, NewLines: 1, Lines: []domain.Line{
			{Kind: domain.Add, NewNo: 1, Text: "package a"},
		}}},
	}}

	got, err := f.svc.FileDiff(context.Background(), ref, mb, c3, change, false)
	if err != nil {
		t.Fatal(err)
	}
	if tok := got.Hunks[0].Lines[0].Tokens; len(tok) != 1 || tok[0] != (highlight.Token{Text: "package a"}) {
		t.Errorf("tokens = %+v", tok)
	}
}

func TestSplitRowsPairDeletionsWithAdditions(t *testing.T) {
	line := func(kind domain.LineKind, old, new int) review.DiffLine {
		return review.DiffLine{Kind: kind, OldNo: old, NewNo: new}
	}
	h := review.DiffHunk{Lines: []review.DiffLine{
		line(domain.Context, 1, 1),
		line(domain.Delete, 2, 0),
		line(domain.Delete, 3, 0),
		line(domain.Delete, 4, 0),
		line(domain.Add, 0, 2),
		line(domain.Context, 5, 3),
		line(domain.Add, 0, 4),
	}}

	got := h.SplitRows()
	// Each row as (left old number, right new number); 0 means empty.
	want := [][2]int{{1, 1}, {2, 2}, {3, 0}, {4, 0}, {5, 3}, {0, 4}}
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d", len(got), len(want))
	}
	for i, row := range got {
		var l, r int
		if row.Left != nil {
			l = row.Left.OldNo
		}
		if row.Right != nil {
			r = row.Right.NewNo
		}
		if [2]int{l, r} != want[i] {
			t.Errorf("row %d = (%d, %d), want %v", i, l, r, want[i])
		}
	}
}
