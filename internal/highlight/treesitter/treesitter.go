// Package treesitter is the Highlighter adapter backed by tree-sitter
// grammars and their highlights.scm queries.
package treesitter

import (
	"embed"
	"log/slog"
	"path"
	"slices"
	"strings"
	"sync"
	"unsafe"

	tsyaml "github.com/tree-sitter-grammars/tree-sitter-yaml/bindings/go"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tsbash "github.com/tree-sitter/tree-sitter-bash/bindings/go"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjson "github.com/tree-sitter/tree-sitter-json/bindings/go"

	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/highlight/treesitter/markdown"
)

//go:embed queries/*.scm
var queries embed.FS

// grammar is a lazily-initialised language and its highlight query.
type grammar struct {
	name     string
	language func() unsafe.Pointer

	once  sync.Once
	lang  *sitter.Language
	query *sitter.Query
}

func (g *grammar) load() bool {
	g.once.Do(func() {
		src, err := queries.ReadFile("queries/" + g.name + ".scm")
		if err != nil {
			slog.Error("highlight query missing", "language", g.name, "err", err)
			return
		}
		lang := sitter.NewLanguage(g.language())
		q, qerr := sitter.NewQuery(lang, string(src))
		if qerr != nil {
			slog.Error("highlight query invalid", "language", g.name, "err", qerr)
			return
		}
		g.lang, g.query = lang, q
	})
	return g.query != nil
}

// Highlighter highlights Go, JSON, YAML, shell and Markdown.
type Highlighter struct {
	byExt map[string]*grammar
}

var _ highlight.Highlighter = (*Highlighter)(nil)

// New returns a Highlighter. Grammars are loaded on first use.
func New() *Highlighter {
	goG := &grammar{name: "go", language: tsgo.Language}
	jsonG := &grammar{name: "json", language: tsjson.Language}
	yamlG := &grammar{name: "yaml", language: tsyaml.Language}
	bashG := &grammar{name: "bash", language: tsbash.Language}
	mdG := &grammar{name: "markdown", language: markdown.Language}
	return &Highlighter{byExt: map[string]*grammar{
		".go":       goG,
		".json":     jsonG,
		".yaml":     yamlG,
		".yml":      yamlG,
		".sh":       bashG,
		".bash":     bashG,
		".md":       mdG,
		".markdown": mdG,
	}}
}

func (h *Highlighter) Highlight(file string, src []byte) [][]highlight.Token {
	g, ok := h.byExt[strings.ToLower(path.Ext(file))]
	if !ok || !g.load() {
		return highlight.Plain(src)
	}
	return tokenise(src, classify(g, src))
}

type capture struct {
	start, end uint
	pattern    uint
	name       string
}

// classify returns the capture name covering each byte of src. Inner nodes
// override the nodes enclosing them, and for the same node the earliest
// pattern in the query wins, as in tree-sitter's own highlighter.
func classify(g *grammar, src []byte) []string {
	parser := sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(g.lang); err != nil {
		return make([]string, len(src))
	}
	tree := parser.Parse(src, nil)
	defer tree.Close()
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()

	names := g.query.CaptureNames()
	var caps []capture
	matches := cursor.Captures(g.query, tree.RootNode(), src)
	for m, i := matches.Next(); m != nil; m, i = matches.Next() {
		c := m.Captures[i]
		caps = append(caps, capture{c.Node.StartByte(), c.Node.EndByte(), uint(m.PatternIndex), names[c.Index]})
	}
	// Paint outer before inner, and later patterns before earlier ones.
	slices.SortStableFunc(caps, func(a, b capture) int {
		if la, lb := a.end-a.start, b.end-b.start; la != lb {
			return int(lb) - int(la)
		}
		return int(b.pattern) - int(a.pattern)
	})
	classes := make([]string, len(src))
	for _, c := range caps {
		for i := c.start; i < c.end && i < uint(len(classes)); i++ {
			classes[i] = c.name
		}
	}
	return classes
}

// tokenise splits src into lines of runs of bytes sharing a class.
func tokenise(src []byte, classes []string) [][]highlight.Token {
	var (
		lines [][]highlight.Token
		line  []highlight.Token
		start int
	)
	flush := func(end int) {
		if end > start {
			line = append(line, highlight.Token{Text: string(src[start:end]), Class: classes[start]})
		}
		start = end
	}
	for i := range src {
		switch {
		case src[i] == '\n':
			flush(i)
			if line == nil {
				line = []highlight.Token{{Text: ""}}
			}
			lines = append(lines, line)
			line, start = nil, i+1
		case i > start && classes[i] != classes[start]:
			flush(i)
		}
	}
	flush(len(src))
	if line != nil {
		lines = append(lines, line)
	}
	return lines
}
