package domain

// CheckState is a Check's outcome so far.
type CheckState string

const (
	CheckPending CheckState = "pending"
	CheckSuccess CheckState = "success"
	CheckFailure CheckState = "failure"
	CheckNeutral CheckState = "neutral" // neutral, skipped or stale
)

// Check is one CI result reported against the PR's head commit.
type Check struct {
	Name     string
	Workflow string // the Actions workflow, when there is one
	State    CheckState
	Required bool // a Required Check must pass before merging
	URL      string
}

// MergeState is whether the PR can be merged, per the code host.
type MergeState string

const (
	MergeClean       MergeState = "clean"
	MergeBlocked     MergeState = "blocked"
	MergeBehind      MergeState = "behind"
	MergeConflicting MergeState = "conflicting"
	MergeUnstable    MergeState = "unstable" // mergeable, but non-required Checks failed
	MergeUnknown     MergeState = "unknown"
)

// Checks are the Checks on the PR's head commit and its MergeState.
type Checks struct {
	Head       SHA
	MergeState MergeState
	Checks     []Check
	PRState    PRState // the code host still reports a MergeState once a PR is closed
}

// ShowMergeState reports whether MergeState is worth showing: not once the
// PR is closed or merged, and not "clean" when there are no Checks, since
// that only means the repository enforces nothing.
func (c Checks) ShowMergeState() bool {
	if c.PRState == Closed || c.PRState == Merged {
		return false
	}
	return c.MergeState != MergeClean || len(c.Checks) > 0
}

// Count returns how many Checks are in state s.
func (c Checks) Count(s CheckState) int {
	n := 0
	for _, ch := range c.Checks {
		if ch.State == s {
			n++
		}
	}
	return n
}
