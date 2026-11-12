package domain

import (
	"strings"
	"testing"
)

func render(lines []Line) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString([]string{" ", "+", "-"}[l.Kind] + l.Text + "\n")
	}
	return b.String()
}

func TestLineDiff(t *testing.T) {
	got := render(LineDiff("a\nb\nc\nd\n", "a\nc\nx\nd"))
	if want := " a\n-b\n c\n+x\n d\n"; got != want {
		t.Errorf("LineDiff =\n%s\nwant\n%s", got, want)
	}
	if got := render(LineDiff("", "new\r\n")); got != "+new\n" {
		t.Errorf("from empty = %q", got)
	}
}

func TestDescriptionChange(t *testing.T) {
	d := Description{
		Original: "one", HasOriginal: true,
		Edits: []DescriptionEdit{{Body: "two"}, {Deleted: true}, {Body: "three"}},
	}
	if got := render(d.Change(0)); got != "-one\n+two\n" {
		t.Errorf("Change(0) = %q", got)
	}
	// Neither side of a deleted revision is known.
	if d.Change(1) != nil || d.Change(2) != nil {
		t.Errorf("Change(1) = %v, Change(2) = %v, want nil", d.Change(1), d.Change(2))
	}
	// Without the Original, the first edit has nothing to diff against.
	d.HasOriginal = false
	if d.Change(0) != nil {
		t.Errorf("Change(0) without Original = %v", d.Change(0))
	}
	// An empty Original is still one.
	d.Original, d.HasOriginal = "", true
	if got := render(d.Change(0)); got != "+two\n" {
		t.Errorf("Change(0) from empty = %q", got)
	}
}
