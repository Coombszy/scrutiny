package views

import (
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
