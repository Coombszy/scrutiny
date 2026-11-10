package github

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
)

// statusTracker records reachability and the rate limits GitHub reports.
// REST and GraphQL have separate limits, so the lower is kept per kind.
type statusTracker struct {
	mu     sync.Mutex
	online bool
	limits map[string]codehost.RateLimit // by X-RateLimit-Resource
}

func (t *statusTracker) failed() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.online = false
}

func (t *statusTracker) observe(h http.Header) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.online = true
	limit, err1 := strconv.Atoi(h.Get("X-RateLimit-Limit"))
	remaining, err2 := strconv.Atoi(h.Get("X-RateLimit-Remaining"))
	if err1 != nil || err2 != nil {
		return
	}
	rl := codehost.RateLimit{Remaining: remaining, Limit: limit}
	if reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		rl.Reset = time.Unix(reset, 0)
	}
	if t.limits == nil {
		t.limits = map[string]codehost.RateLimit{}
	}
	t.limits[h.Get("X-RateLimit-Resource")] = rl
}

func (t *statusTracker) status() codehost.Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := codehost.Status{Online: t.online}
	for _, rl := range t.limits {
		if st.RateLimit.Limit == 0 || rl.Remaining*st.RateLimit.Limit < st.RateLimit.Remaining*rl.Limit {
			st.RateLimit = rl
		}
	}
	return st
}
