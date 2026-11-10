package treesitter_test

import (
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/highlight/treesitter"
)

func classOf(line []highlight.Token, text string) (string, bool) {
	for _, tok := range line {
		if tok.Text == text {
			return tok.Class, true
		}
	}
	return "", false
}

func TestGoTokensAreClassified(t *testing.T) {
	src := "package main\n\nfunc greet() string {\n\treturn \"hi\" // done\n}\n"
	lines := treesitter.New().Highlight("cmd/main.go", []byte(src))

	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5", len(lines))
	}
	for _, c := range []struct {
		line       int
		text, want string
	}{
		{0, "package", "keyword"},
		{2, "func", "keyword"},
		{2, "greet", "function"},
		{2, "string", "type"},
		{3, `"hi"`, "string"},
		{3, "// done", "comment"},
	} {
		got, ok := classOf(lines[c.line], c.text)
		if !ok || got != c.want {
			t.Errorf("line %d %q: class %q (found %v), want %q; tokens %+v", c.line+1, c.text, got, ok, c.want, lines[c.line])
		}
	}
}

func TestTokensReassembleIntoTheSource(t *testing.T) {
	src := "package main\n\nvar s = `multi\nline`\n"
	lines := treesitter.New().Highlight("x.go", []byte(src))

	var rebuilt []string
	for _, l := range lines {
		var b strings.Builder
		for _, tok := range l {
			b.WriteString(tok.Text)
		}
		rebuilt = append(rebuilt, b.String())
	}
	if got := strings.Join(rebuilt, "\n") + "\n"; got != src {
		t.Errorf("reassembled %q, want %q", got, src)
	}
	if cls, _ := classOf(lines[3], "line`"); cls != "string" {
		t.Errorf("second line of raw string: class %q, want string; tokens %+v", cls, lines[3])
	}
}

func TestEachSupportedLanguageHighlights(t *testing.T) {
	for path, src := range map[string]string{
		"a.json":        `{"key": true}`,
		"a.yaml":        "key: value # note\n",
		".github/x.yml": "on: push\n",
		"run.sh":        "echo \"hi\"\n",
		"README.md":     "# Title\n\nText\n",
	} {
		lines := treesitter.New().Highlight(path, []byte(src))
		highlighted := false
		for _, l := range lines {
			for _, tok := range l {
				highlighted = highlighted || tok.Class != ""
			}
		}
		if !highlighted {
			t.Errorf("%s: no highlighted tokens in %+v", path, lines)
		}
	}
}

func TestUnsupportedLanguageIsPlain(t *testing.T) {
	lines := treesitter.New().Highlight("main.rs", []byte("fn main() {}\n"))
	if len(lines) != 1 || len(lines[0]) != 1 || lines[0][0] != (highlight.Token{Text: "fn main() {}"}) {
		t.Errorf("Highlight = %+v, want one plain line", lines)
	}
}
