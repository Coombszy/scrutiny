package web_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/settings"
)

func TestThemeCSSFollowsTheSetting(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodGet, "/theme.css", nil)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q", ct)
	}
	mustContain(t, rec.Body.String(), "--bg0: #282828;", ".hl-keyword")

	rec = a.do(t, http.MethodPost, "/settings/theme", url.Values{"theme": {"gruvbox-light"}})
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("status %d, HX-Refresh %q", rec.Code, rec.Header().Get("HX-Refresh"))
	}
	mustContain(t, a.do(t, http.MethodGet, "/theme.css", nil).Body.String(), "--bg0: #fbf1c7;")

	if rec := a.do(t, http.MethodPost, "/settings/theme", url.Values{"theme": {"nope"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown theme: status %d, want 400", rec.Code)
	}
}

func TestDropInThemesAreOffered(t *testing.T) {
	a := newApp(t)
	os.WriteFile(filepath.Join(a.themesDir, "solar.toml"), []byte("name = \"Solarized\"\n[ui]\nbg0 = \"#fdf6e3\"\n"), 0o644)

	body := a.do(t, http.MethodGet, "/", nil).Body.String()
	mustContain(t, body, `data-themes=`, "Solarized", `href="/theme.css?v=gruvbox-dark"`)
	if rec := a.do(t, http.MethodPost, "/settings/theme", url.Values{"theme": {"solar"}}); rec.Code != http.StatusNoContent {
		t.Errorf("drop-in theme: status %d", rec.Code)
	}
}

func TestRebindingAHotkey(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, "/settings/keys", url.Values{"action": {"next_comment"}, "binding": {"ctrl+n"}})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if cfg, _ := a.settings.Load(); cfg.Keys["next_comment"] != "ctrl+n" || cfg.Keys["refresh"] != "r" {
		t.Errorf("Keys = %v", cfg.Keys)
	}
	mustContain(t, a.do(t, http.MethodGet, "/", nil).Body.String(), `data-keys=`, `ctrl+n`)

	for _, bad := range []url.Values{
		{"action": {"fly"}, "binding": {"f"}},
		{"action": {"refresh"}, "binding": {"ctrl+"}},
	} {
		if rec := a.do(t, http.MethodPost, "/settings/keys", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("%v: status %d, want 400", bad, rec.Code)
		}
	}
	if cfg, _ := a.settings.Load(); cfg.Keys["refresh"] != "r" {
		t.Error("a rejected binding was saved")
	}
}

func TestPRPageShowsStackNavigationAndSinceLastReview(t *testing.T) {
	a := newApp(t)
	st := a.host.State(ref)
	st.Stack = &domain.Stack{Position: 2, Size: 3, Entries: []domain.StackPR{
		{Number: 6, Title: "Lay foundations", Position: 1, State: domain.Merged},
		{Number: 7, Title: "This one", Position: 2},
		{Number: 9, Title: "Build on it", Position: 3},
	}}
	st.LastReviewed = c1

	body := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, body,
		"Stack 2/3",
		`id="stack-parent" href="/github.com/o/r/pull/6"`, "Lay foundations",
		`id="stack-child" href="/github.com/o/r/pull/9"`, "Build on it",
		`id="since-last-review"`, "first="+string(c2)+"&amp;last="+string(c2),
		`data-view-mode="split"`,
	)
}

func TestSettingsKeysRoundTripThroughTheConfig(t *testing.T) {
	cfg := settings.Defaults()
	cfg.Keys["refresh"] = "shift+r"
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
