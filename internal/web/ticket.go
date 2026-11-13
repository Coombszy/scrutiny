package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"html"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ticketLife = 30 * time.Second
	maxTickets = 32
)

// tickets are Goto Tickets: short-lived, single-use passes that let the
// Userscript open a PR page in GitHub's own tab. That navigation comes from
// github.com, so Secure would refuse it as cross-site. A web page can ask for
// a ticket but cannot read the answer (it has no CORS headers); only the
// Userscript, whose requests CORS does not bind, can.
type tickets struct {
	mu     sync.Mutex
	issued map[string]time.Time // ticket -> expiry
}

func newTickets() *tickets {
	return &tickets{issued: map[string]time.Time{}}
}

// issue returns a new ticket, dropping expired ones and, past maxTickets, the
// one closest to expiry.
func (t *tickets) issue(now time.Time) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	tk := hex.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	var oldest string
	for k, exp := range t.issued {
		if !now.Before(exp) {
			delete(t.issued, k)
		} else if oldest == "" || exp.Before(t.issued[oldest]) {
			oldest = k
		}
	}
	if len(t.issued) >= maxTickets {
		delete(t.issued, oldest)
	}
	t.issued[tk] = now.Add(ticketLife)
	return tk
}

// redeem reports whether tk is an unexpired ticket, using it up.
func (t *tickets) redeem(tk string, now time.Time) bool {
	if tk == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, exp := range t.issued {
		if subtle.ConstantTimeCompare([]byte(k), []byte(tk)) == 1 {
			delete(t.issued, k)
			return now.Before(exp)
		}
	}
	return false
}

// isTicketRequest reports whether r asks for a Goto Ticket. Like the health
// check it skips the Origin and cross-site checks.
func isTicketRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/ticket"
}

func (t *tickets) serveIssue(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"ticket":"` + t.issue(time.Now()) + `"}`))
}

// serveBounce answers a ticketed cross-site navigation with a page that sends
// the tab on to the same URL without the ticket. That second navigation
// comes from Scrutiny's own page, so it is same-origin, and so are reloads of
// the page it lands on.
func serveBounce(w http.ResponseWriter, r *http.Request, csp string) {
	u := *r.URL
	q := u.Query()
	q.Del("ticket")
	u.RawQuery = q.Encode()
	u.Scheme, u.Host = "", ""
	// A path the browser would read as another host ("//evil.example" or
	// "/\evil.example") goes to the Dashboard instead.
	if !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") || strings.HasPrefix(u.Path, "/\\") {
		u.Path, u.RawPath, u.RawQuery = "/", "", ""
	}
	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	dest := html.EscapeString(u.RequestURI())
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>Scrutiny</title>` +
		`<script src="/static/bounce.js" data-to="` + dest + `"></script>` +
		`<noscript><a href="` + dest + `">Continue to Scrutiny</a></noscript>`))
}
