// Package highlight is the Highlighter port: source code to highlighted
// tokens.
package highlight

import "strings"

// Token is a run of text on one line. Class is the highlight capture name
// (e.g. "keyword", "function.method"), or "" for plain text.
type Token struct {
	Text  string
	Class string
}

// Highlighter splits a file's source into lines of tokens.
type Highlighter interface {
	// Highlight returns one slice of tokens per line of src; line N of the
	// file is element N-1. Files in unsupported languages come back as plain
	// text. Concatenating a line's tokens gives the line back unchanged.
	Highlight(path string, src []byte) [][]Token
}

// Plain returns src as untokenised lines.
func Plain(src []byte) [][]Token {
	text := strings.TrimSuffix(string(src), "\n")
	if text == "" && len(src) == 0 {
		return nil
	}
	lines := strings.Split(text, "\n")
	out := make([][]Token, len(lines))
	for i, l := range lines {
		out[i] = []Token{{Text: l}}
	}
	return out
}

// PlainHighlighter highlights nothing.
type PlainHighlighter struct{}

func (PlainHighlighter) Highlight(_ string, src []byte) [][]Token { return Plain(src) }
