package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"time"
)

// contentSecurityPolicy only allows Scrutiny's own scripts, so HTML from
// comment bodies cannot run script even if the code host's sanitising fails.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' https: data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

// devContentSecurityPolicy also allows inline scripts, which a live-reload
// proxy (air, for `just run-live`) injects. Never use it outside development.
const devContentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' https: data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

// NewCSRFToken returns a random per-process CSRF token.
func NewCSRFToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Secure rejects requests that did not come from a page served on localhost:
// the Host and Origin headers must name a loopback host (a DNS-rebinding
// defence), the browser must not mark the request as coming from another
// site (unless a GET carries a Goto Ticket from GET /ticket), and every
// mutating request must carry the CSRF token in the X-CSRF-Token header or a
// csrf_token form field.
func Secure(next http.Handler, csrfToken string) http.Handler {
	return secure(next, csrfToken, contentSecurityPolicy)
}

func secure(next http.Handler, csrfToken, csp string) http.Handler {
	t := newTickets()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if isHealthCheck(r) {
			next.ServeHTTP(w, r)
			return
		}
		if isTicketRequest(r) {
			t.serveIssue(w)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !isLoopback(u.Host) {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
		}
		if !sameOrigin(r) {
			if r.Method == http.MethodGet && t.redeem(r.URL.Query().Get("ticket"), time.Now()) {
				serveBounce(w, r, csp)
				return
			}
			http.Error(w, "forbidden cross-site request: open this page from Scrutiny", http.StatusForbidden)
			return
		}
		if isMutating(r.Method) {
			got := r.Header.Get("X-CSRF-Token")
			if got == "" {
				got = r.PostFormValue("csrf_token")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(csrfToken)) != 1 {
				http.Error(w, "missing or invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func isLoopback(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	if host == "localhost" {
		return true
	}
	if len(host) > 1 && host[0] == '[' && host[len(host)-1] == ']' {
		host = host[1 : len(host)-1]
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameOrigin reports whether the browser says r came from Scrutiny's own
// pages, or from the user (a typed URL, a bookmark, another app). Opening a
// PR page fetches it and starts Language Servers on its code, so another
// site, including one on another localhost port, must not be able to load
// one with a GET. Only the Dashboard may be linked to from elsewhere.
// Clients that send no Sec-Fetch-Site header are not browsers acting for a
// web page.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return r.Method == http.MethodGet && r.URL.Path == "/"
}

// isHealthCheck reports whether r is a GET of /health, which the Userscript
// on github.com (whose requests may carry its own extension Origin) uses to
// see whether Scrutiny is running. It says only that, so it skips the Origin
// and cross-site checks; the Host check still applies.
func isHealthCheck(r *http.Request) bool {
	return r.Method == http.MethodGet && r.URL.Path == "/health"
}

func isMutating(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}
	return true
}
