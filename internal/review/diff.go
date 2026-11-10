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
	// Unplaced are the file's Review Threads that cannot be shown on a line
	// of this diff: outdated, outside its hunks, or from another Comparison.
	Unplaced []domain.ReviewThread
}

// DiffHunk is a hunk of highlighted lines.
type DiffHunk struct {
	Header string // e.g. "@@ -1,2 +1,3 @@ func f()"
	Lines  []DiffLine
}

// DiffLine is one highlighted diff line.
type DiffLine struct {
	Kind    domain.LineKind
	OldNo   int
	NewNo   int
	Tokens  []highlight.Token
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
// correctly, and placing the file's Review Threads on their lines.
func (s *Service) FileDiff(ctx context.Context, ref domain.PRRef, base, head domain.SHA, change domain.FileChange) (DiffFile, error) {
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
		out.Hunks[i] = dh
	}
	out.place(snap, base, head)
	return out, nil
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
