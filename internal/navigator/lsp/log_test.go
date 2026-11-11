package lsp

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/navigator"
)

func TestTheLogKeepsTheNewestEntries(t *testing.T) {
	r := newLogRing()
	for i := range logCapacity + 5 {
		r.add(navigator.LogDebug, "stderr", strconv.Itoa(i))
	}
	got := r.list()
	if len(got) != logCapacity || got[0].Text != "5" || got[len(got)-1].Text != strconv.Itoa(logCapacity+4) {
		t.Errorf("kept %d entries, %q to %q", len(got), got[0].Text, got[len(got)-1].Text)
	}
}

func TestTheLogTruncatesLongLines(t *testing.T) {
	r := newLogRing()
	r.add(navigator.LogDebug, "stderr", strings.Repeat("x", logLineMax*2))
	if n := len(r.list()[0].Text); n > logLineMax+len("…") {
		t.Errorf("kept %d bytes", n)
	}
}

func TestTheLastProblemIsSinceTheServerStarted(t *testing.T) {
	r := newLogRing()
	now := time.Now()
	r.now = func() time.Time { return now }
	r.add(navigator.LogError, "server", "old")
	now = now.Add(time.Minute)
	r.add(navigator.LogInfo, "server", "fine")
	if p := r.lastProblem(now); p != "" {
		t.Errorf("lastProblem = %q, want none since then", p)
	}
	r.add(navigator.LogWarn, "server", "new")
	r.add(navigator.LogInfo, "server", "fine again")
	if p := r.lastProblem(now); p != "new" {
		t.Errorf("lastProblem = %q, want new", p)
	}
}

func TestLinesWrittenInPiecesArePassedOnWhole(t *testing.T) {
	var lines []string
	w := &lineWriter{line: func(s string) { lines = append(lines, s) }}
	w.Write([]byte("one\r\ntw"))
	w.Write([]byte("o\n\nthr"))
	w.flush()
	if strings.Join(lines, "|") != "one|two|thr" {
		t.Errorf("lines = %q", lines)
	}
}
