package review_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
)

// withLongFile gives a.go 200 lines at head, changed in two places: line 11
// added, and old line 100 rewritten as new line 101.
func (f *fixture) withLongFile() domain.FileChange {
	var head []string
	for i := 1; i <= 200; i++ {
		head = append(head, fmt.Sprintf("line %d", i))
	}
	f.repos.Blobs[c3] = map[string]string{"a.go": strings.Join(head, "\n") + "\n"}
	change := domain.FileChange{Path: "a.go", OldPath: "a.go", Status: domain.Modified, Additions: 2, Deletions: 1}
	f.repos.ChangeSets[repostore.Pair{From: mb, To: c3}] = []domain.FileChange{change}
	f.repos.Diffs[repostore.Pair{From: mb, To: c3}] = map[string]domain.FileDiff{"a.go": {FileChange: change, Hunks: []domain.Hunk{
		{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 2, Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 10, NewNo: 10, Text: "line 10"},
			{Kind: domain.Add, NewNo: 11, Text: "line 11"},
		}},
		{OldStart: 99, OldLines: 3, NewStart: 100, NewLines: 3, Lines: []domain.Line{
			{Kind: domain.Context, OldNo: 99, NewNo: 100, Text: "line 100"},
			{Kind: domain.Delete, OldNo: 100, Text: "old 100"},
			{Kind: domain.Add, NewNo: 101, Text: "line 101"},
			{Kind: domain.Context, OldNo: 101, NewNo: 102, Text: "line 102"},
		}},
	}}}
	return change
}

// loadPage opens the PR page over whatever diff the test set up.
func (f *fixture) loadPage(t *testing.T) {
	t.Helper()
	if _, err := f.svc.PRPage(context.Background(), ref, review.ComparisonRequest{}); err != nil {
		t.Fatal(err)
	}
}

func TestDiffsKnowTheUnchangedLinesTheyLeaveOut(t *testing.T) {
	f := newFixture(t)
	change := f.withLongFile()
	f.loadPage(t)

	d, err := f.svc.FileDiff(context.Background(), ref, mb, c3, change, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []*review.Gap{
		{Start: 1, End: 9, Offset: 0, First: true},
		{Start: 12, End: 99, Offset: -1},
	}
	for i, h := range d.Hunks {
		if h.Gap == nil || *h.Gap != *want[i] {
			t.Errorf("gap above hunk %d = %+v, want %+v", i, h.Gap, want[i])
		}
	}
	if tail := (review.Gap{Start: 103, End: 200, Offset: -1, Last: true}); d.TailGap == nil || *d.TailGap != tail {
		t.Errorf("tail gap = %+v, want %+v", d.TailGap, tail)
	}
}

func TestContextLinesAreUnchangedLinesWithBothNumbers(t *testing.T) {
	f := newFixture(t)
	f.withLongFile()
	f.loadPage(t)

	got, err := f.svc.ContextLines(context.Background(), ref, c3, "a.go", review.Gap{Start: 12, End: 14, Offset: -1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%d lines, want 3", len(got))
	}
	for i, l := range got {
		if l.Kind != domain.Context || l.NewNo != 12+i || l.OldNo != 11+i || l.Tokens[0].Text != fmt.Sprintf("line %d", 12+i) {
			t.Errorf("line %d = %+v", i, l)
		}
	}
}

func TestRevealingAGapAFewLinesAtATime(t *testing.T) {
	g := review.Gap{Start: 12, End: 199, Offset: -1, First: false, Last: true}
	for _, c := range []struct {
		dir        review.Direction
		shown      review.Gap
		rest       *review.Gap
		restBefore bool
	}{
		{review.Down, review.Gap{Start: 12, End: 75, Offset: -1}, &review.Gap{Start: 76, End: 199, Offset: -1, Last: true}, false},
		{review.Up, review.Gap{Start: 136, End: 199, Offset: -1, Last: true}, &review.Gap{Start: 12, End: 135, Offset: -1}, true},
		{review.All, g, nil, false},
	} {
		shown, rest, before := g.Reveal(c.dir, 64)
		if shown != c.shown || (rest == nil) != (c.rest == nil) || (rest != nil && *rest != *c.rest) || before != c.restBefore {
			t.Errorf("%s: shown %+v, rest %+v (before %v)", c.dir, shown, rest, before)
		}
	}
	small := review.Gap{Start: 1, End: 64, First: true}
	if shown, rest, _ := small.Reveal(review.Up, 64); shown != small || rest != nil {
		t.Errorf("a 64-line gap should be revealed whole: %+v, %+v", shown, rest)
	}
}

func TestFullFileFillsEveryGapAndPlacesThreadsThere(t *testing.T) {
	f := newFixture(t)
	change := f.withLongFile()
	f.host.State(ref).Threads = []domain.ReviewThread{{ID: "far", Path: "a.go", Line: 50, Side: domain.Right}}
	f.loadPage(t)

	d, err := f.svc.FileDiff(context.Background(), ref, mb, c3, change, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Hunks) != 1 || d.Hunks[0].Gap != nil || d.TailGap != nil {
		t.Fatalf("full file = %d hunks, tail %+v", len(d.Hunks), d.TailGap)
	}
	lines := d.Hunks[0].Lines
	if len(lines) != 201 { // 200 head lines and one deletion
		t.Fatalf("%d lines, want 201", len(lines))
	}
	if l := lines[49]; l.NewNo != 50 || l.OldNo != 49 || l.Kind != domain.Context || len(l.Threads) != 1 {
		t.Errorf("line 50 = %+v, want context with its thread", l)
	}
	if len(d.Unplaced) != 0 {
		t.Errorf("Unplaced = %v", d.Unplaced)
	}
}

func TestTheRevealStepIsConfigurable(t *testing.T) {
	g := review.Gap{Start: 1, End: 100, First: true}
	shown, rest, _ := g.Reveal(review.Down, 10)
	if shown.End != 10 || rest == nil || rest.Start != 11 {
		t.Errorf("Reveal(10) shown %+v, rest %+v", shown, rest)
	}
}
