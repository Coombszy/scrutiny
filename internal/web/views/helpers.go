package views

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

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
