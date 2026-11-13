package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/web"
)

const token = "s3cret"

func secured() http.Handler {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	return web.Secure(ok, token)
}

func TestSecureRejectsNonLocalHost(t *testing.T) {
	for host, want := range map[string]int{
		"127.0.0.1:7878":    http.StatusOK,
		"localhost:7878":    http.StatusOK,
		"[::1]:7878":        http.StatusOK,
		"evil.example:7878": http.StatusForbidden,
		"localhost.evil":    http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		secured().ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %q: status %d, want %d", host, rec.Code, want)
		}
	}
}

func TestSecureRejectsForeignOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:7878"
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	secured().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status %d, want 403", rec.Code)
	}
}

func TestSecureRejectsCrossSiteRequests(t *testing.T) {
	pr := "/github.com/o/r/pull/1"
	cases := []struct {
		path, site string
		want       int
	}{
		{pr, "", http.StatusOK},            // not a browser
		{pr, "none", http.StatusOK},        // typed, bookmarked or opened by another app
		{pr, "same-origin", http.StatusOK}, // Scrutiny's own pages
		{pr, "same-site", http.StatusForbidden},
		{pr, "cross-site", http.StatusForbidden},
		{"/open?url=https://github.com/o/r/pull/1", "cross-site", http.StatusForbidden},
		{"/static/scrutiny.js", "cross-site", http.StatusForbidden},
		{"/", "cross-site", http.StatusOK},       // the Dashboard may be linked to
		{"/health", "cross-site", http.StatusOK}, // the Userscript checks Scrutiny is running
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		req.Host = "127.0.0.1:7878"
		if c.site != "" {
			req.Header.Set("Sec-Fetch-Site", c.site)
		}
		rec := httptest.NewRecorder()
		secured().ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("GET %s with Sec-Fetch-Site %q: status %d, want %d", c.path, c.site, rec.Code, c.want)
		}
	}
}

func TestHealthAnswersAnyOriginOnALoopbackHost(t *testing.T) {
	h := web.NewServer(web.Deps{CSRFToken: token}).Handler()
	for host, want := range map[string]int{
		"127.0.0.1:7878":    http.StatusOK,
		"evil.example:7878": http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Host = host
		req.Header.Set("Origin", "chrome-extension://abcdef")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %q: status %d, want %d", host, rec.Code, want)
		}
		if want == http.StatusOK && !strings.Contains(rec.Body.String(), `"app":"scrutiny"`) {
			t.Errorf("body %q does not name Scrutiny", rec.Body.String())
		}
	}
}

func TestSecureRequiresCSRFTokenOnMutatingRequests(t *testing.T) {
	cases := []struct {
		name   string
		header string
		form   string
		want   int
	}{
		{"missing", "", "", http.StatusForbidden},
		{"wrong header", "nope", "", http.StatusForbidden},
		{"header", token, "", http.StatusOK},
		{"form field", "", "csrf_token=" + token, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.form))
			req.Host = "127.0.0.1:7878"
			req.Header.Set("Origin", "http://127.0.0.1:7878")
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if c.header != "" {
				req.Header.Set("X-CSRF-Token", c.header)
			}
			rec := httptest.NewRecorder()
			secured().ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Errorf("status %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestOnlyDevModeAllowsInlineScripts(t *testing.T) {
	for dev, want := range map[bool]bool{false: false, true: true} {
		h := web.NewServer(web.Deps{CSRFToken: token, Dev: dev}).Handler()
		req := httptest.NewRequest(http.MethodGet, "/static/scrutiny.css", nil)
		req.Host = "127.0.0.1:7878"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := strings.Contains(rec.Header().Get("Content-Security-Policy"), "'unsafe-inline'; style-src"); got != want {
			t.Errorf("dev=%v: inline scripts allowed = %v, want %v (CSP %q)", dev, got, want, rec.Header().Get("Content-Security-Policy"))
		}
	}
}

func ticket(t *testing.T, h http.Handler) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ticket", nil)
	req.Host = "127.0.0.1:7878"
	req.Header.Set("Origin", "chrome-extension://abcdef")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body struct{ Ticket string }
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Ticket == "" {
		t.Fatalf("GET /ticket: status %d, body %q", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("GET /ticket must not let web pages read the ticket")
	}
	return body.Ticket
}

func TestGotoTicketLetsOneCrossSiteNavigationThrough(t *testing.T) {
	h := secured()
	tk := ticket(t, h)
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "127.0.0.1:7878"
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/github.com/o/r/pull/1?ticket=" + tk + "&comment=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("with a ticket: status %d, want 200", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `data-to="/github.com/o/r/pull/1?comment=5"`) || strings.Contains(body, tk) {
		t.Errorf("bounce page should send the tab on without the ticket: %q", body)
	}
	if rec := get("/github.com/o/r/pull/1?ticket=" + tk); rec.Code != http.StatusForbidden {
		t.Errorf("reused ticket: status %d, want 403", rec.Code)
	}
	if rec := get("/github.com/o/r/pull/1?ticket=forged"); rec.Code != http.StatusForbidden {
		t.Errorf("forged ticket: status %d, want 403", rec.Code)
	}
}

func TestGotoTicketNeverSendsTheTabToAnotherHost(t *testing.T) {
	h := secured()
	for _, path := range []string{"//evil.example/x", `/\evil.example/x`, "/%2Fevil.example/x"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = path
		req.URL.RawQuery = "ticket=" + ticket(t, h)
		req.Host = "127.0.0.1:7878"
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		body := rec.Body.String()
		to := body[strings.Index(body, `data-to="`)+len(`data-to="`):]
		to = to[:strings.Index(to, `"`)]
		if strings.HasPrefix(to, "//") || strings.HasPrefix(to, `/\`) {
			t.Errorf("path %q bounces to %q, another host", path, to)
		}
	}
}
