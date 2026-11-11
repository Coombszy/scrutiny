package review_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

// diffOf loads a one-hunk diff of a.go made of the given lines.
func diffOf(t *testing.T, lines ...domain.Line) review.DiffHunk {
	t.Helper()
	f := newFixture(t)
	change := domain.FileChange{Path: "a.go", OldPath: "a.go", Status: domain.Modified}
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = []domain.FileChange{change}
	f.repos.Diffs[repostore.Pair{From: mb, To: c3}] = map[string]domain.FileDiff{"a.go": {FileChange: change, Hunks: []domain.Hunk{{Lines: lines}}}}
	f.loadPage(t)
	d, err := f.svc.FileDiff(context.Background(), ref, mb, c3, change, false)
	if err != nil {
		t.Fatal(err)
	}
	return d.Hunks[0]
}

// marked renders a line with its changed parts in [brackets].
func marked(l review.DiffLine) string {
	var b strings.Builder
	for i, tok := range l.Tokens {
		if l.Changed != nil && l.Changed[i] {
			b.WriteString("[" + tok.Text + "]")
		} else {
			b.WriteString(tok.Text)
		}
	}
	return b.String()
}

func TestChangedWordsAreMarkedWithinAReplacedLine(t *testing.T) {
	h := diffOf(t,
		domain.Line{Kind: domain.Delete, OldNo: 1, Text: "total := add(price, tax)"},
		domain.Line{Kind: domain.Add, NewNo: 1, Text: "total := add(price, vat, fee)"},
	)
	if got := marked(h.Lines[0]); got != "total := add(price, [tax])" {
		t.Errorf("deleted line = %q", got)
	}
	if got := marked(h.Lines[1]); got != "total := add(price, [vat, fee])" {
		t.Errorf("added line = %q", got)
	}
}

func TestLinesPairInOrderWithinARun(t *testing.T) {
	h := diffOf(t,
		domain.Line{Kind: domain.Delete, OldNo: 1, Text: "a := 1"},
		domain.Line{Kind: domain.Delete, OldNo: 2, Text: "b := 2"},
		domain.Line{Kind: domain.Add, NewNo: 1, Text: "a := 10"},
	)
	if got := marked(h.Lines[0]); got != "a := [1]" {
		t.Errorf("first deletion = %q", got)
	}
	if h.Lines[1].Changed != nil {
		t.Errorf("an unpaired deletion was marked: %q", marked(h.Lines[1]))
	}
	if got := marked(h.Lines[2]); got != "a := [10]" {
		t.Errorf("addition = %q", got)
	}
}

func TestRewrittenLinesAreNotMarked(t *testing.T) {
	h := diffOf(t,
		domain.Line{Kind: domain.Delete, OldNo: 1, Text: "return errors.New(\"boom\")"},
		domain.Line{Kind: domain.Add, NewNo: 1, Text: "for _, x := range items {"},
	)
	for _, l := range h.Lines {
		if l.Changed != nil {
			t.Errorf("marked a rewritten line: %q", marked(l))
		}
	}
}

func TestMarksSplitHighlightTokensAndKeepTheirClasses(t *testing.T) {
	f := newFixture(t)
	f.svc.Highlighter = keywordHighlighter{}
	change := domain.FileChange{Path: "a.go", OldPath: "a.go", Status: domain.Modified}
	f.repos.Blobs[mb] = map[string]string{"a.go": "var x = 1\n"}
	f.repos.Blobs[c3] = map[string]string{"a.go": "var x = 2\n"}
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = []domain.FileChange{change}
	f.repos.Diffs[repostore.Pair{From: mb, To: c3}] = map[string]domain.FileDiff{"a.go": {FileChange: change, Hunks: []domain.Hunk{{Lines: []domain.Line{
		{Kind: domain.Delete, OldNo: 1, Text: "var x = 1"}, {Kind: domain.Add, NewNo: 1, Text: "var x = 2"},
	}}}}}
	f.loadPage(t)
	d, err := f.svc.FileDiff(context.Background(), ref, mb, c3, change, false)
	if err != nil {
		t.Fatal(err)
	}
	add := d.Hunks[0].Lines[1]
	want := []highlight.Token{{Text: "var", Class: "keyword"}, {Text: " x = "}, {Text: "2"}}
	if len(add.Tokens) != len(want) {
		t.Fatalf("tokens = %+v", add.Tokens)
	}
	for i := range want {
		if add.Tokens[i] != want[i] || add.Changed[i] != (i == 2) {
			t.Errorf("token %d = %+v changed=%v", i, add.Tokens[i], add.Changed[i])
		}
	}
}

// keywordHighlighter marks a leading "var" as a keyword.
type keywordHighlighter struct{}

func (keywordHighlighter) Highlight(_ string, src []byte) [][]highlight.Token {
	lines := highlight.Plain(src)
	for i, l := range lines {
		if rest, ok := strings.CutPrefix(l[0].Text, "var"); ok {
			lines[i] = []highlight.Token{{Text: "var", Class: "keyword"}, {Text: rest}}
		}
	}
	return lines
}
