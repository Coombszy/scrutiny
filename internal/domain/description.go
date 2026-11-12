package domain

import (
	"strings"
	"time"
)

// Description is a PR's description: its body, and the edits made to it,
// shown in the Conversation.
type Description struct {
	Author    string
	BodyHTML  string // rendered and sanitised by the code host
	CreatedAt time.Time
	// Original is the body as first written, as Markdown, when HasOriginal;
	// it is unknown when older edits were not read.
	Original    string
	HasOriginal bool
	Edits       []DescriptionEdit // oldest first
}

// DescriptionEdit is one edit to a PR's description.
type DescriptionEdit struct {
	Editor   string
	EditedAt time.Time
	// Body is the description after the edit, as Markdown; empty when the
	// revision was deleted from its history.
	Body    string
	Deleted bool
}

// Change is the line diff an edit made, or nil when either side is unknown:
// the first edit read without the Original, or a revision deleted from the
// history.
func (d Description) Change(i int) []Line {
	before := d.Original
	if i > 0 {
		before = d.Edits[i-1].Body
		if d.Edits[i-1].Deleted {
			return nil
		}
	}
	e := d.Edits[i]
	if e.Deleted || (i == 0 && !d.HasOriginal) {
		return nil
	}
	return LineDiff(before, e.Body)
}

// LineDiff is the line diff from a to b, as Context, Delete and Add lines
// by the longest common subsequence of lines.
func LineDiff(a, b string) []Line {
	x, y := splitLines(a), splitLines(b)
	// lcs[i][j] is the length of the longest common subsequence of x[i:]
	// and y[j:].
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []Line
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			out = append(out, Line{Kind: Context, OldNo: i + 1, NewNo: j + 1, Text: x[i]})
			i, j = i+1, j+1
		case j == len(y) || (i < len(x) && lcs[i+1][j] >= lcs[i][j+1]):
			out = append(out, Line{Kind: Delete, OldNo: i + 1, Text: x[i]})
			i++
		default:
			out = append(out, Line{Kind: Add, NewNo: j + 1, Text: y[j]})
			j++
		}
	}
	return out
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
