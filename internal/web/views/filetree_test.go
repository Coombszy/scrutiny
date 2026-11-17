package views

import (
	"reflect"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

// shape flattens a File Tree to "dir/" and "file#index" entries, indented by depth.
func shape(d *treeDir, indent string) []string {
	var out []string
	for _, c := range d.Dirs {
		out = append(out, indent+c.Name+"/")
		out = append(out, shape(c, indent+"  ")...)
	}
	for _, f := range d.Files {
		out = append(out, indent+f.Name+"#"+itoa(f.Index))
	}
	return out
}

func entry(path string) review.FileEntry {
	return review.FileEntry{FileChange: domain.FileChange{Path: path, OldPath: path, Status: domain.Modified}}
}

func TestFileTreeKeepsThePagesOrderAndMergesSingleFolderChains(t *testing.T) {
	files := []review.FileEntry{ // in review.TreeOrder, as on the page
		entry("cmd/scrutiny/main.go"),
		entry("internal/web/static/scrutiny.css"),
		entry("internal/web/views/diff.templ"),
		entry("internal/web/views/pr.templ"),
		entry("README.md"),
		entry("go.mod"),
	}
	want := []string{
		"cmd/scrutiny/",
		"  main.go#0",
		"internal/web/",
		"  static/",
		"    scrutiny.css#1",
		"  views/",
		"    diff.templ#2",
		"    pr.templ#3",
		"README.md#4",
		"go.mod#5",
	}
	if got := shape(fileTree(files), ""); !reflect.DeepEqual(got, want) {
		t.Errorf("tree =\n%v\nwant\n%v", got, want)
	}
}

func TestFileTreeRowsMarkEachChangeStatus(t *testing.T) {
	for status, icon := range map[domain.ChangeStatus]string{domain.Added: "+", domain.Deleted: "−", domain.Modified: "•", domain.Renamed: "→"} {
		if got := treeStatusIcon(status); got != icon {
			t.Errorf("%s: icon %q, want %q", status, got, icon)
		}
	}
	renamed := review.FileEntry{FileChange: domain.FileChange{Path: "lex/new.go", OldPath: "lex/old.go", Status: domain.Renamed}}
	if got, want := treeFileTitle(renamed), "renamed: lex/old.go → lex/new.go"; got != want {
		t.Errorf("title %q, want %q", got, want)
	}
}
