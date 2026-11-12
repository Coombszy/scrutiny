package web_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestTopBarOffersNewVersion(t *testing.T) {
	a := newApp(t)
	if body := a.do(t, http.MethodGet, "/", nil).Body.String(); strings.Contains(body, "new-version") {
		t.Fatal("the top bar offers a New Version before any was found")
	}
	a.release.Set("v0.3.0")
	a.checkRelease()
	body := a.do(t, http.MethodGet, "/settings", nil).Body.String()
	if !strings.Contains(body, "v0.3.0 available") || !strings.Contains(body, "/releases/tag/v0.3.0") {
		t.Fatalf("the top bar does not offer v0.3.0:\n%s", body)
	}
}
