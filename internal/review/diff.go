package review

import (
	"context"
	"fmt"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
)

// DiffFile is a file's diff ready to render.
type DiffFile struct {
	domain.FileChange
	Hunks []DiffHunk
	// TailGap is the unchanged lines after the last hunk, if any.
	TailGap *Gap
	// Full is set when every unchanged line is shown.
	Full bool
	// Unplaced are the file's Review Threads that cannot be shown on a line
	// of this diff: outdated, outside its hunks, or from another Comparison.
	Unplaced []domain.ReviewThread
}

// DiffHunk is a hunk of highlighted lines.
type DiffHunk struct {
	Header string // e.g. "@@ -1,2 +1,3 @@ func f()"; empty for a whole file
	Lines  []DiffLine
	// Gap is the unchanged lines between the previous hunk (or the start of
	// the file) and this one, if any.
	Gap *Gap
}

// Gap is a run of unchanged lines a diff leaves out, numbered as at head.
type Gap struct {
	Start, End int  // head line numbers, inclusive
	Offset     int  // base line number minus head line number, the same throughout
	First      bool // the gap starts at the top of the file
	Last       bool // the gap runs to the end of the file
}

// Size is the number of lines in the gap.
func (g Gap) Size() int { return g.End - g.Start + 1 }

// Direction is which end of a Gap to reveal lines from.
type Direction string

const (
	Down Direction = "down" // below the hunk above
	Up   Direction = "up"   // above the hunk below
	All  Direction = "all"
)

// Reveal splits off the lines to show: step lines from one end (the
// diff.expand_lines setting), or the whole gap when it is no bigger than
// that. It returns what remains hidden, if anything, and whether that sits
// before the shown lines.
func (g Gap) Reveal(dir Direction, step int) (shown Gap, rest *Gap, restBefore bool) {
	if dir == All || g.Size() <= step {
		return g, nil, false
	}
	shown, remaining := g, g
	if dir == Up {
		shown.Start, shown.First = g.End-step+1, false
		remaining.End, remaining.Last = shown.Start-1, false
		return shown, &remaining, true
	}
	shown.End, shown.Last = g.Start+step-1, false
	remaining.Start, remaining.First = shown.End+1, false
	return shown, &remaining, false
}

// DiffLine is one highlighted diff line.
type DiffLine struct {
	Kind   domain.LineKind
	OldNo  int
	NewNo  int
	Tokens []highlight.Token
	// Changed marks, token by token, what differs from the line this one
	// replaces (or is replaced by); nil when nothing is marked.
	Changed []bool
	Threads []domain.ReviewThread // anchored to this line
}

// SplitRow is one row of the Split View Mode. Either side may be nil.
type SplitRow struct {
	Left, Right *DiffLine
}

// SplitRows lays the hunk out side by side: context lines appear on both
// sides, and each run of deletions is paired line by line with the run of
// additions that follows it.
func (h DiffHunk) SplitRows() []SplitRow {
	var rows []SplitRow
	lines := h.Lines
	for i := 0; i < len(lines); {
		if lines[i].Kind == domain.Context {
			rows = append(rows, SplitRow{Left: &lines[i], Right: &lines[i]})
			i++
			continue
		}
		var dels, adds []*DiffLine
		for ; i < len(lines) && lines[i].Kind == domain.Delete; i++ {
			dels = append(dels, &lines[i])
		}
		for ; i < len(lines) && lines[i].Kind == domain.Add; i++ {
			adds = append(adds, &lines[i])
		}
		for j := 0; j < max(len(dels), len(adds)); j++ {
			var row SplitRow
			if j < len(dels) {
				row.Left = dels[j]
			}
			if j < len(adds) {
				row.Right = adds[j]
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// FileDiff loads one file's diff between base and head, highlighting each
// side from the whole file at that commit so multi-line constructs colour
// correctly, and placing the file's Review Threads on their lines. With
// full, every unchanged line is shown too.
func (s *Service) FileDiff(ctx context.Context, ref domain.PRRef, base, head domain.SHA, change domain.FileChange, full bool) (DiffFile, error) {
	repo := ref.Repo
	snap, err := s.snapshot(ctx, ref)
	if err != nil {
		return DiffFile{}, err
	}
	fd, err := s.Repos.FileDiff(ctx, repo, base, head, change)
	if err != nil {
		return DiffFile{}, err
	}
	var oldLines, newLines [][]highlight.Token
	if change.Status != domain.Added {
		oldLines = s.highlightBlob(ctx, repo, base, change.OldPath)
	}
	if change.Status != domain.Deleted {
		newLines = s.highlightBlob(ctx, repo, head, change.Path)
	}
	out := DiffFile{FileChange: change, Hunks: make([]DiffHunk, len(fd.Hunks))}
	for i, h := range fd.Hunks {
		dh := DiffHunk{Header: header(h), Lines: make([]DiffLine, len(h.Lines))}
		for j, l := range h.Lines {
			tokens := tokensAt(newLines, l.NewNo)
			if l.Kind == domain.Delete {
				tokens = tokensAt(oldLines, l.OldNo)
			}
			if tokens == nil {
				tokens = []highlight.Token{{Text: l.Text}}
			}
			dh.Lines[j] = DiffLine{Kind: l.Kind, OldNo: l.OldNo, NewNo: l.NewNo, Tokens: tokens}
		}
		markChanges(dh.Lines)
		out.Hunks[i] = dh
	}
	// Only files on both sides have unchanged lines to reveal.
	if (change.Status == domain.Modified || change.Status == domain.Renamed) && newLines != nil {
		out.findGaps(fd.Hunks, len(newLines))
		if full {
			out.fill(newLines)
		}
	}
	out.place(snap, base, head)
	return out, nil
}

// findGaps records the unchanged lines around each hunk. A hunk with no
// lines on a side sits after its start line on that side.
func (d *DiffFile) findGaps(hunks []domain.Hunk, lines int) {
	first := func(start, count int) int {
		if count == 0 {
			return start + 1
		}
		return start
	}
	nextNew, nextOld := 1, 1
	for i, h := range hunks {
		newFirst, oldFirst := first(h.NewStart, h.NewLines), first(h.OldStart, h.OldLines)
		if newFirst > nextNew {
			d.Hunks[i].Gap = &Gap{Start: nextNew, End: newFirst - 1, Offset: oldFirst - newFirst, First: i == 0}
		}
		nextNew, nextOld = newFirst+h.NewLines, oldFirst+h.OldLines
	}
	if nextNew <= lines {
		d.TailGap = &Gap{Start: nextNew, End: lines, Offset: nextOld - nextNew, Last: true}
	}
}

// fill merges the hunks into one, with every gap's lines in place.
func (d *DiffFile) fill(newLines [][]highlight.Token) {
	var all []DiffLine
	for _, h := range d.Hunks {
		if h.Gap != nil {
			all = append(all, contextLines(newLines, *h.Gap)...)
		}
		all = append(all, h.Lines...)
	}
	if d.TailGap != nil {
		all = append(all, contextLines(newLines, *d.TailGap)...)
	}
	d.Hunks, d.TailGap, d.Full = []DiffHunk{{Lines: all}}, nil, true
}

func contextLines(newLines [][]highlight.Token, g Gap) []DiffLine {
	var out []DiffLine
	for n := g.Start; n <= g.End; n++ {
		tokens := tokensAt(newLines, n)
		if tokens == nil {
			tokens = []highlight.Token{{}}
		}
		out = append(out, DiffLine{Kind: domain.Context, OldNo: n + g.Offset, NewNo: n, Tokens: tokens})
	}
	return out
}

// ContextLines returns the unchanged lines of a Gap in path at head,
// highlighted, for expanding a diff.
func (s *Service) ContextLines(ctx context.Context, ref domain.PRRef, head domain.SHA, path string, g Gap) ([]DiffLine, error) {
	if !validPath(path) {
		return nil, fmt.Errorf("%w: %q", ErrBadPath, path)
	}
	src, err := s.Repos.Blob(ctx, ref.Repo, head, path)
	if err != nil {
		return nil, err
	}
	newLines := s.Highlighter.Highlight(path, src)
	if g.Start < 1 || g.End > len(newLines) || g.Start > g.End {
		return nil, fmt.Errorf("%w: lines %d-%d of %s", ErrBadPath, g.Start, g.End, path)
	}
	return contextLines(newLines, g), nil
}

// place attaches each of the file's threads to the line it is anchored to.
// Thread line numbers refer to the PR's head (RIGHT) and merge base (LEFT),
// so they only line up when this diff has the same sides.
func (d *DiffFile) place(snap snapshot, base, head domain.SHA) {
	for _, t := range snap.State.Threads {
		if t.Path != d.Path {
			continue
		}
		var line *DiffLine
		if t.Line != 0 && head == snap.Head && (t.Side == domain.Right || base == snap.MergeBase) {
			line = d.lineAt(t.Side, t.Line)
		}
		if line == nil {
			d.Unplaced = append(d.Unplaced, t)
			continue
		}
		line.Threads = append(line.Threads, t)
	}
}

func (d *DiffFile) lineAt(side domain.DiffSide, n int) *DiffLine {
	for i := range d.Hunks {
		for j := range d.Hunks[i].Lines {
			l := &d.Hunks[i].Lines[j]
			if side == domain.Right && l.Kind != domain.Delete && l.NewNo == n ||
				side == domain.Left && l.Kind != domain.Add && l.OldNo == n {
				return l
			}
		}
	}
	return nil
}

func (s *Service) highlightBlob(ctx context.Context, repo domain.Repo, sha domain.SHA, path string) [][]highlight.Token {
	src, err := s.Repos.Blob(ctx, repo, sha, path)
	if err != nil {
		return nil
	}
	return s.Highlighter.Highlight(path, src)
}

func tokensAt(lines [][]highlight.Token, lineNo int) []highlight.Token {
	if lineNo < 1 || lineNo > len(lines) {
		return nil
	}
	return lines[lineNo-1]
}

func header(h domain.Hunk) string {
	s := fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	if h.Section != "" {
		s += " " + h.Section
	}
	return s
}
