package web_test

import (
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
		{"/", "cross-site", http.StatusOK}, // the Dashboard may be linked to
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
