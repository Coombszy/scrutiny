package lsp

import (
	"sync"
	"time"

	"github.com/Coombszy/scrutiny/internal/navigator"
)

const (
	logCapacity = 300  // entries kept per executable
	logLineMax  = 2048 // bytes kept of each entry's text
)

// logRing keeps an executable's most recent log entries, across the
// processes and Workspaces it has run for.
type logRing struct {
	mu      sync.Mutex
	entries []navigator.LogEntry // oldest first once full: entries[next:] then entries[:next]
	next    int
	full    bool
	now     func() time.Time
}

func newLogRing() *logRing { return &logRing{now: time.Now} }

func (r *logRing) add(level navigator.LogLevel, source, text string) {
	if len(text) > logLineMax {
		text = text[:logLineMax] + "…"
	}
	e := navigator.LogEntry{Time: r.now(), Level: level, Source: source, Text: text}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		r.entries = append(r.entries, e)
		if len(r.entries) == logCapacity {
			r.full = true
		}
		return
	}
	r.entries[r.next] = e
	r.next = (r.next + 1) % logCapacity
}

// list returns the entries, oldest first.
func (r *logRing) list() []navigator.LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]navigator.LogEntry, 0, len(r.entries))
	out = append(out, r.entries[r.next:]...)
	return append(out, r.entries[:r.next]...)
}

// lastProblem returns the newest error or warning's text since since.
func (r *logRing) lastProblem(since time.Time) string {
	entries := r.list()
	for i := len(entries) - 1; i >= 0 && !entries[i].Time.Before(since); i-- {
		if entries[i].Problem() {
			return entries[i].Text
		}
	}
	return ""
}
