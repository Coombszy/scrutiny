package gitcli

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Coombszy/scrutiny/internal/domain"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@ ?(.*)$`)

// parseHunks parses the hunks of a single-file unified diff, ignoring the
// file header lines before the first @@.
func parseHunks(diff string) ([]domain.Hunk, error) {
	var (
		hunks        []domain.Hunk
		cur          *domain.Hunk
		oldNo, newNo int
	)
	for _, line := range strings.Split(strings.TrimSuffix(diff, "\n"), "\n") {
		if strings.HasPrefix(line, "@@ ") {
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("malformed hunk header %q", line)
			}
			h := domain.Hunk{
				OldStart: atoi(m[1]), OldLines: count(m[2]),
				NewStart: atoi(m[3]), NewLines: count(m[4]),
				Section: m[5],
			}
			hunks = append(hunks, h)
			cur = &hunks[len(hunks)-1]
			oldNo, newNo = h.OldStart, h.NewStart
			continue
		}
		if cur == nil || line == "" {
			continue
		}
		text := line[1:]
		switch line[0] {
		case ' ':
			cur.Lines = append(cur.Lines, domain.Line{Kind: domain.Context, OldNo: oldNo, NewNo: newNo, Text: text})
			oldNo++
			newNo++
		case '-':
			cur.Lines = append(cur.Lines, domain.Line{Kind: domain.Delete, OldNo: oldNo, Text: text})
			oldNo++
		case '+':
			cur.Lines = append(cur.Lines, domain.Line{Kind: domain.Add, NewNo: newNo, Text: text})
			newNo++
		}
		// "\ No newline at end of file" and anything else is skipped.
	}
	return hunks, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// count parses an optional hunk line count, which defaults to 1.
func count(s string) int {
	if s == "" {
		return 1
	}
	return atoi(s)
}
