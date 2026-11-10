package views

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

// tokenClass turns a highlight capture name such as "function.method" into
// CSS classes for it and each of its parents: "hl-function hl-function-method".
func tokenClass(capture string) string {
	parts := strings.Split(capture, ".")
	classes := make([]string, len(parts))
	for i := range parts {
		classes[i] = "hl-" + strings.Join(parts[:i+1], "-")
	}
	return strings.Join(classes, " ")
}

// FileURL is where a file's diff fragment is loaded from.
func FileURL(pr domain.PRRef, cmp review.Comparison, f domain.FileChange) string {
	q := url.Values{
		"base":   {string(cmp.Base)},
		"head":   {string(cmp.Head)},
		"path":   {f.Path},
		"old":    {f.OldPath},
		"status": {string(f.Status)},
	}
	return pr.Path() + "/file?" + q.Encode()
}

func comparisonURL(pr domain.PRRef, first, last domain.SHA) string {
	return pr.Path() + "?" + url.Values{"first": {string(first)}, "last": {string(last)}}.Encode()
}

func lineNo(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func kindClass(k domain.LineKind) string {
	switch k {
	case domain.Add:
		return "add"
	case domain.Delete:
		return "del"
	}
	return "ctx"
}

func fileID(i int) string { return "file-" + strconv.Itoa(i) }

func itoa(n int) string { return strconv.Itoa(n) }

// ThreadAnchor is where a new Review Thread is being added.
type ThreadAnchor struct {
	Base, Head, Path string
	Line             int
	Side             string
}

// newThreadURL is the new-thread form's URL without its side and line,
// which the add-comment button appends.
func newThreadURL(pr domain.PRRef, base, head domain.SHA, path string, cols int) string {
	q := url.Values{"base": {string(base)}, "head": {string(head)}, "path": {path}, "cols": {strconv.Itoa(cols)}}
	return pr.Path() + "/threads/new?" + q.Encode()
}

func boolString(b bool) string { return strconv.FormatBool(b) }

func threadLocation(t domain.ReviewThread) string {
	if t.Line == 0 {
		return t.Path
	}
	return t.Path + ":" + strconv.Itoa(t.Line)
}

func reactionVals(subject string, r domain.Reaction, on bool) string {
	b, _ := json.Marshal(map[string]string{"subject": subject, "content": string(r), "on": strconv.FormatBool(on)})
	return string(b)
}

func hasReacted(groups []domain.ReactionGroup, r domain.Reaction) bool {
	for _, g := range groups {
		if g.Content == r {
			return g.ViewerHasReacted
		}
	}
	return false
}

func pendingCount(p *domain.PendingReview) string {
	if p == nil {
		return "0"
	}
	return strconv.Itoa(p.Comments)
}

func unresolvedThreads(ts []domain.ReviewThread) int {
	n := 0
	for _, t := range ts {
		if !t.Resolved {
			n++
		}
	}
	return n
}

// viewedVals sends the file's path; htmx adds the checkbox's own value only
// when it is checked, so an absent "viewed" means unviewed.
func viewedVals(path string) string {
	b, _ := json.Marshal(map[string]string{"path": path})
	return string(b)
}

// unifiedSide is the side a comment on a unified diff line goes on: deleted
// lines are LEFT, everything else RIGHT.
func unifiedSide(l review.DiffLine) domain.DiffSide {
	if l.Kind == domain.Delete {
		return domain.Left
	}
	return domain.Right
}

func unifiedLine(l review.DiffLine) int {
	if l.Kind == domain.Delete {
		return l.OldNo
	}
	return l.NewNo
}

// rowThreads are the threads anchored to either side of a split row; a
// context line appears on both sides but its threads only once.
func rowThreads(row review.SplitRow) []domain.ReviewThread {
	var ts []domain.ReviewThread
	if row.Left != nil {
		ts = append(ts, row.Left.Threads...)
	}
	if row.Right != nil && row.Right != row.Left {
		ts = append(ts, row.Right.Threads...)
	}
	return ts
}

func mergeStateLabel(m domain.MergeState) string {
	switch m {
	case domain.MergeClean:
		return "Ready to merge"
	case domain.MergeBlocked:
		return "Blocked"
	case domain.MergeBehind:
		return "Behind base"
	case domain.MergeConflicting:
		return "Conflicts"
	case domain.MergeUnstable:
		return "Unstable"
	}
	return "Merge state unknown"
}

func checkIcon(s domain.CheckState) string {
	switch s {
	case domain.CheckSuccess:
		return "✓"
	case domain.CheckFailure:
		return "✗"
	case domain.CheckPending:
		return "●"
	}
	return "–"
}

func checkName(c domain.Check) string {
	if c.Workflow != "" {
		return c.Workflow + " / " + c.Name
	}
	return c.Name
}

// sortedChecks lists failures first, then pending, then the rest, with
// Required Checks ahead within each group.
func sortedChecks(cs []domain.Check) []domain.Check {
	rank := map[domain.CheckState]int{domain.CheckFailure: 0, domain.CheckPending: 1, domain.CheckSuccess: 2, domain.CheckNeutral: 3}
	out := slices.Clone(cs)
	slices.SortStableFunc(out, func(a, b domain.Check) int {
		if d := rank[a.State] - rank[b.State]; d != 0 {
			return d
		}
		switch {
		case a.Required && !b.Required:
			return -1
		case b.Required && !a.Required:
			return 1
		}
		return 0
	})
	return out
}

// blobURL is the file view of path at the PR's head, at line if positive.
func blobURL(pr domain.PRRef, path string, line int) string {
	u := pr.Path() + "/blob?" + url.Values{"path": {path}}.Encode()
	if line > 0 {
		u += "#L" + strconv.Itoa(line)
	}
	return u
}

// BlobURL is blobURL for handlers.
func BlobURL(pr domain.PRRef, path string, line int) string { return blobURL(pr, path, line) }

func markerTitle(m review.Marker) string {
	switch m {
	case review.MarkerAdded:
		return "Added by this pull request"
	case review.MarkerRemoved:
		return "Removed by this pull request"
	}
	return ""
}

func usages(n int) string {
	if n == 1 {
		return "1 usage"
	}
	return strconv.Itoa(n) + " usages"
}

// groupByPath groups references by file, keeping their order.
func groupByPath(refs []review.Reference) [][]review.Reference {
	var groups [][]review.Reference
	index := map[string]int{}
	for _, r := range refs {
		i, ok := index[r.Path]
		if !ok {
			i = len(groups)
			index[r.Path] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], r)
	}
	return groups
}

// stackURL is another PR in the same repository's Stack.
func stackURL(pr domain.PRRef, number int) string {
	return domain.PRRef{Repo: pr.Repo, Number: number}.Path()
}

// ago describes how long before now t was, for status lines.
func ago(t time.Time) string {
	switch d := time.Since(t); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return t.Local().Format("2 Jan 15:04")
}

func commentCount(n int) string {
	if n == 1 {
		return "1 comment"
	}
	return strconv.Itoa(n) + " comments"
}

// checksOutcome is the worst state among the Checks.
func checksOutcome(c domain.Checks) domain.CheckState {
	switch {
	case c.Count(domain.CheckFailure) > 0:
		return domain.CheckFailure
	case c.Count(domain.CheckPending) > 0:
		return domain.CheckPending
	}
	return domain.CheckSuccess
}

// checksSummary is e.g. "All 17 Checks passed" or "1 failing · 2 pending · 14 passed".
func checksSummary(c domain.Checks) string {
	failing, pending, passed := c.Count(domain.CheckFailure), c.Count(domain.CheckPending), c.Count(domain.CheckSuccess)
	if failing == 0 && pending == 0 {
		if len(c.Checks) == 1 {
			return "All 1 Check passed"
		}
		return fmt.Sprintf("All %d Checks passed", len(c.Checks))
	}
	var parts []string
	for _, p := range []struct {
		n    int
		what string
	}{{failing, "failing"}, {pending, "pending"}, {passed, "passed"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.what))
		}
	}
	return strings.Join(parts, " · ")
}

func mergeIcon(m domain.MergeState) string {
	switch m {
	case domain.MergeClean:
		return "✓"
	case domain.MergeBlocked, domain.MergeConflicting:
		return "✗"
	}
	return "●"
}

// byReviewers is e.g. "Approved by ana and bo", or just what when nobody is named.
func byReviewers(what string, names []string) string {
	switch len(names) {
	case 0:
		return what
	case 1:
		return what + " by " + names[0]
	}
	return what + " by " + strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
