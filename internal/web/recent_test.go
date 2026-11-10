package web_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestForgettingARecentPRRemovesItFromTheDashboard(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	mustContain(t, a.do(t, http.MethodGet, "/", nil).Body.String(),
		`hx-post="`+prPath+`/forget"`, `hx-target="closest li"`)

	rec := a.do(t, http.MethodPost, prPath+"/forget", formValues())
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("status %d, body %q; want an empty 200 to remove the item", rec.Code, rec.Body.String())
	}
	if body := a.do(t, http.MethodGet, "/", nil).Body.String(); strings.Contains(body, prPath+"/forget") {
		t.Error("the PR is still in Recently opened")
	}
	if !slices.Contains(a.host.Forgotten, ref) {
		t.Errorf("code host forgot %v", a.host.Forgotten)
	}
}

func TestForgetNeedsTheCSRFToken(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	req := a.do(t, http.MethodPost, prPath+"/forget", nil)
	if req.Code == http.StatusOK {
		t.Error("forgot a PR without the CSRF token")
	}
}
