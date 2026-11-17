package views

import (
	"strings"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

// treeDir is a folder in the File Tree. Name is its path below its parent:
// a folder whose only child is a folder is merged into it, so Name can hold
// several segments, e.g. "internal/web/views".
type treeDir struct {
	Name  string
	Dirs  []*treeDir
	Files []treeFile
}

// treeFile is a changed file in the File Tree; Index is its place in the
// page's files, which names its section.
type treeFile struct {
	Name  string
	Index int
	File  review.FileEntry
}

// fileTree arranges files, already in review.TreeOrder, into folders. Files
// sit at Path, which for a deletion is its old path.
func fileTree(files []review.FileEntry) *treeDir {
	root := &treeDir{}
	for i, f := range files {
		segs := strings.Split(f.Path, "/")
		dir := root
		for _, seg := range segs[:len(segs)-1] {
			dir = dir.child(seg)
		}
		dir.Files = append(dir.Files, treeFile{Name: segs[len(segs)-1], Index: i, File: f})
	}
	root.tidy()
	return root
}

func (d *treeDir) child(name string) *treeDir {
	for _, c := range d.Dirs {
		if c.Name == name {
			return c
		}
	}
	c := &treeDir{Name: name}
	d.Dirs = append(d.Dirs, c)
	return c
}

// tidy merges single-folder chains below d.
func (d *treeDir) tidy() {
	for i, c := range d.Dirs {
		for len(c.Files) == 0 && len(c.Dirs) == 1 {
			only := c.Dirs[0]
			c = &treeDir{Name: c.Name + "/" + only.Name, Dirs: only.Dirs, Files: only.Files}
		}
		c.tidy()
		d.Dirs[i] = c
	}
}

// treeStatusIcon is the File Tree's mark for how a file changed.
func treeStatusIcon(s domain.ChangeStatus) string {
	switch s {
	case domain.Added:
		return "+"
	case domain.Deleted:
		return "−"
	case domain.Renamed:
		return "→"
	default:
		return "•"
	}
}

// treeFileTitle is a File Tree row's tooltip: the path, or both paths of a
// rename.
func treeFileTitle(f review.FileEntry) string {
	if f.Status == domain.Renamed && f.OldPath != f.Path {
		return string(f.Status) + ": " + f.OldPath + " → " + f.Path
	}
	return string(f.Status) + ": " + f.Path
}
