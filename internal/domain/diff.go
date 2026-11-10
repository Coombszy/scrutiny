package domain

// ChangeStatus is how a file changed between the two sides of a Comparison.
type ChangeStatus string

const (
	Added    ChangeStatus = "added"
	Modified ChangeStatus = "modified"
	Deleted  ChangeStatus = "deleted"
	Renamed  ChangeStatus = "renamed"
)

// FileChange summarises one file's change, without its hunks.
type FileChange struct {
	Path      string // path on the right-hand side (the left-hand side for deletions)
	OldPath   string // path on the left-hand side; differs from Path only for renames
	Status    ChangeStatus
	Additions int
	Deletions int
	Binary    bool
}

// ChangedLines is the total of added and deleted lines.
func (f FileChange) ChangedLines() int { return f.Additions + f.Deletions }

// FileDiff is a file's change with its hunks.
type FileDiff struct {
	FileChange
	Hunks []Hunk
}

// Hunk is a contiguous region of a FileDiff.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Section            string // text after the closing @@, usually the enclosing function
	Lines              []Line
}

// LineKind is whether a diff line is unchanged, added or deleted.
type LineKind int

const (
	Context LineKind = iota
	Add
	Delete
)

// Line is one line of a Hunk. OldNo is zero for added lines and NewNo is zero
// for deleted lines.
type Line struct {
	Kind  LineKind
	OldNo int
	NewNo int
	Text  string
}
