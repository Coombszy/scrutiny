package review

import (
	"unicode"
	"unicode/utf8"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
)

// Limits on marking changes within lines: beyond these a line is left
// whole, as marking costs more than it helps.
const (
	inlineMaxLen   = 1000 // bytes per line
	inlineMinShare = 0.5  // of the two lines' text that must be in common
)

// markChanges pairs each run of deleted lines with the run of added lines
// after it, line by line (as the Split View Mode lays them out), and marks
// the words that differ within each pair.
func markChanges(lines []DiffLine) {
	for i := 0; i < len(lines); {
		if lines[i].Kind != domain.Delete {
			i++
			continue
		}
		dels := i
		for i < len(lines) && lines[i].Kind == domain.Delete {
			i++
		}
		adds := i
		for i < len(lines) && lines[i].Kind == domain.Add {
			i++
		}
		for j := 0; j < min(adds-dels, i-adds); j++ {
			markPair(&lines[dels+j], &lines[adds+j])
		}
	}
}

// markPair marks the differing words of a deleted line and the added line
// replacing it, unless they have too little in common to be "the same
// line, changed".
func markPair(del, add *DiffLine) {
	oldText, newText := text(del.Tokens), text(add.Tokens)
	if len(oldText) > inlineMaxLen || len(newText) > inlineMaxLen || oldText == newText {
		return
	}
	a, b := words(oldText), words(newText)
	keepA, keepB, common := lcs(a, b)
	if float64(2*common)/float64(len(oldText)+len(newText)) < inlineMinShare {
		return
	}
	del.Tokens, del.Changed = mark(del.Tokens, changedRanges(a, keepA))
	add.Tokens, add.Changed = mark(add.Tokens, changedRanges(b, keepB))
}

func text(tokens []highlight.Token) string {
	var s string
	for _, t := range tokens {
		s += t.Text
	}
	return s
}

// words splits s into identifiers, runs of spaces, and single other
// characters: the units changes are marked in.
func words(s string) []string {
	var out []string
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		j := i + size
		kind := wordKind(r)
		if kind != 0 {
			for j < len(s) {
				r2, size2 := utf8.DecodeRuneInString(s[j:])
				if wordKind(r2) != kind {
					break
				}
				j += size2
			}
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// wordKind groups runes that join into one word: 1 for identifier
// characters, 2 for spaces, 0 for anything that stands alone.
func wordKind(r rune) int {
	switch {
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	case unicode.IsSpace(r):
		return 2
	}
	return 0
}

// lcs finds a longest common subsequence of a and b, matching as early as
// possible so changes come out as contiguous runs, and returns which words
// of each are kept and how many bytes they share.
func lcs(a, b []string) (keepA, keepB []bool, common int) {
	// suffix[i][j] is the LCS length of a[i:] and b[j:].
	suffix := make([][]int, len(a)+1)
	for i := range suffix {
		suffix[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				suffix[i][j] = suffix[i+1][j+1] + 1
			} else {
				suffix[i][j] = max(suffix[i+1][j], suffix[i][j+1])
			}
		}
	}
	keepA, keepB = make([]bool, len(a)), make([]bool, len(b))
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] == b[j]:
			keepA[i], keepB[j] = true, true
			common += len(a[i])
			i, j = i+1, j+1
		case suffix[i+1][j] >= suffix[i][j+1]:
			i++
		default:
			j++
		}
	}
	return keepA, keepB, common
}

// span is a byte range [start, end).
type span struct{ start, end int }

// changedRanges are the byte ranges of the words not kept, with adjacent
// ones merged.
func changedRanges(ws []string, keep []bool) []span {
	var out []span
	pos := 0
	for i, w := range ws {
		if !keep[i] {
			if n := len(out); n > 0 && out[n-1].end == pos {
				out[n-1].end += len(w)
			} else {
				out = append(out, span{pos, pos + len(w)})
			}
		}
		pos += len(w)
	}
	return out
}

// mark splits tokens at the edges of the changed ranges, keeping their
// highlight classes, and reports which of the resulting tokens changed.
func mark(tokens []highlight.Token, changed []span) ([]highlight.Token, []bool) {
	if len(changed) == 0 {
		return tokens, nil
	}
	var (
		out   []highlight.Token
		flags []bool
		pos   int
	)
	inChange := func(at int) (bool, int) { // whether at is changed, and where that run ends
		for _, s := range changed {
			if at < s.start {
				return false, s.start
			}
			if at < s.end {
				return true, s.end
			}
		}
		return false, 1 << 30
	}
	for _, t := range tokens {
		for rest := t.Text; rest != ""; {
			isChanged, until := inChange(pos)
			n := min(len(rest), until-pos)
			out = append(out, highlight.Token{Text: rest[:n], Class: t.Class})
			flags = append(flags, isChanged)
			rest, pos = rest[n:], pos+n
		}
	}
	return out, flags
}
