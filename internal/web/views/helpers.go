package views

import (
	"encoding/json"
	"fmt"
	"github.com/Coombszy/scrutiny/internal/navigator"
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
	// StartLine and StartSide begin a multi-line range ending at Line;
	// StartLine is zero for a single line.
	StartLine int
	StartSide string
}

// Location names the anchor's file and lines as a thread's header does,
// e.g. "parser.go:7" or "parser.go:3-7"; a range across sides marks each
// end with - or +, e.g. "parser.go:-3 to +7".
func (a ThreadAnchor) Location() string {
	switch {
	case a.StartLine == 0:
		return a.Path + ":" + strconv.Itoa(a.Line)
	case a.StartSide != a.Side:
		return a.Path + ":-" + strconv.Itoa(a.StartLine) + " to +" + strconv.Itoa(a.Line)
	default:
		return a.Path + ":" + strconv.Itoa(a.StartLine) + "-" + strconv.Itoa(a.Line)
	}
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
	if t.StartLine != 0 && t.StartLine != t.Line {
		return t.Path + ":" + strconv.Itoa(t.StartLine) + "-" + strconv.Itoa(t.Line)
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

// contextOldLine is a context line's number in the old file, or zero.
func contextOldLine(l review.DiffLine) int {
	if l.Kind == domain.Context {
		return l.OldNo
	}
	return 0
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

func pendingComments(n int) string {
	if n == 1 {
		return "1 pending comment"
	}
	return strconv.Itoa(n) + " pending comments"
}

func pendingLocation(c review.PendingComment) string {
	if c.Line == 0 {
		return c.Path
	}
	return c.Path + ":" + strconv.Itoa(c.Line)
}

// excerpt is the start of a comment's Markdown on one line.
func excerpt(body string) string {
	s := strings.Join(strings.Fields(body), " ")
	if r := []rune(s); len(r) > 80 {
		return string(r[:79]) + "…"
	}
	return s
}

// contextURL loads a gap's unchanged lines, revealing them in dir; header
// is the hunk header the gap's controls sit on, if any.
func contextURL(c DiffContext, g review.Gap, dir review.Direction, header string) string {
	q := url.Values{
		"base": {string(c.Base)}, "head": {string(c.Head)},
		"path": {c.File.Path}, "old": {c.File.OldPath}, "status": {string(c.File.Status)},
		"start": {strconv.Itoa(g.Start)}, "end": {strconv.Itoa(g.End)}, "offset": {strconv.Itoa(g.Offset)},
		"first": {strconv.FormatBool(g.First)}, "last": {strconv.FormatBool(g.Last)}, "dir": {string(dir)},
	}
	if header != "" {
		q.Set("header", header)
	}
	return c.PR.Path() + "/context?" + q.Encode()
}

// lspOverall sums the PR's Language Servers up for the icon: failed if
// any failed, busy while any is starting or indexing, ready when every
// available one is ready, otherwise idle (stopped, none or not installed).
func lspOverall(langs []review.LanguageStatus) string {
	ready, busy, failed := 0, false, false
	available := 0
	for _, l := range langs {
		if l.Unavailable {
			continue
		}
		available++
		switch {
		case l.Phase == navigator.Failed:
			failed = true
		case l.Busy():
			busy = true
		case l.Phase == navigator.Ready:
			ready++
		}
	}
	switch {
	case failed:
		return "failed"
	case busy:
		return "busy"
	case available > 0 && ready == available:
		return "ready"
	}
	return "idle"
}

// lspTooltip says what each language's server is doing, one per line.
func lspTooltip(langs []review.LanguageStatus) string {
	if len(langs) == 0 {
		return "No Language Server is needed for this pull request's files"
	}
	lines := make([]string, len(langs))
	for i, l := range langs {
		name := languageName(l.Language)
		if l.Unavailable {
			lines[i] = name + ": no Language Server installed (see Settings)"
			continue
		}
		server := l.Server
		if server == "" {
			server = "Language Server"
		}
		var doing string
		switch l.Phase {
		case navigator.Starting:
			doing = "starting…"
		case navigator.Indexing:
			doing = "indexing…"
		case navigator.Ready:
			doing = "ready"
		case navigator.Failed:
			doing = "failed: " + l.Err
		default:
			doing = "not running (starts when needed)"
		}
		lines[i] = name + " · " + server + ": " + doing
	}
	return strings.Join(lines, "\n")
}

// languageName is a language id as people write it.
func languageName(id string) string {
	names := map[string]string{
		"go": "Go", "typescript": "TypeScript", "javascript": "JavaScript", "python": "Python", "rust": "Rust",
		"java": "Java", "ruby": "Ruby", "c": "C", "cpp": "C++", "csharp": "C#", "lua": "Lua", "bash": "Bash",
		"yaml": "YAML", "json": "JSON", "terraform": "Terraform", "zig": "Zig",
	}
	if n, ok := names[id]; ok {
		return n
	}
	return id
}

// thousands writes n with comma separators, e.g. 4,812.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// sparkH and sparkY size a bar of a 20-unit-high spark chart; any count
// shows at least a sliver.
func sparkH(n, max int) string {
	h := float64(n) / float64(max) * 20
	if h < 1.5 {
		h = 1.5
	}
	return strconv.FormatFloat(h, 'f', 2, 64)
}

func sparkY(n, max int) string {
	h, _ := strconv.ParseFloat(sparkH(n, max), 64)
	return strconv.FormatFloat(20-h, 'f', 2, 64)
}

// rateLimitName names a GitHub rate limit resource for people.
func rateLimitName(resource string) string {
	switch resource {
	case "", "core":
		return "API"
	case "graphql":
		return "GraphQL"
	}
	return resource
}
