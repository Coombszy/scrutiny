package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/settings"
)

func settingsForm(change func(url.Values)) url.Values {
	v := url.Values{
		"theme":              {"gruvbox-dark"},
		"view_mode":          {"split"},
		"collapse_threshold": {"1000"},
		"port":               {"7878"},
	}
	for action, binding := range settings.DefaultKeys() {
		v.Set("key."+action, binding)
	}
	if change != nil {
		change(v)
	}
	return v
}

func TestTopBarLinksToSettingsWithACog(t *testing.T) {
	a := newApp(t)
	for _, path := range []string{"/", prPath, "/settings"} {
		body := a.do(t, http.MethodGet, path, nil).Body.String()
		mustContain(t, body, `href="/settings"`, `aria-label="Settings"`, `class="cog"`)
		if strings.Contains(body, `>Language Servers</a>`) {
			t.Errorf("%s: top bar still has a Language Servers text link", path)
		}
	}
}

func TestSettingsPageShowsEverySettingAndTheLanguageServers(t *testing.T) {
	a := newApp(t)
	body := a.do(t, http.MethodGet, "/settings", nil).Body.String()
	mustContain(t, body,
		`name="theme"`, `name="view_mode"`, `name="collapse_threshold"`, `name="port"`,
		`id="language-servers"`, `id="server-gopls"`, `hx-get="/settings/servers/list"`,
	)
	for action := range settings.DefaultKeys() {
		mustContain(t, body, `name="key.`+action+`"`)
	}
}

func TestOldServersPageRedirectsToSettings(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodGet, "/settings/servers", nil)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings#language-servers" {
		t.Errorf("status %d, Location %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestSavingSettingsWritesTheConfig(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, "/settings", settingsForm(func(v url.Values) {
		v.Set("view_mode", "unified")
		v.Set("collapse_threshold", "250")
		v.Set("port", "9000")
		v.Set("key.refresh", "shift+r")
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	mustContain(t, rec.Body.String(), "Saved")
	if rec.Header().Get("HX-Refresh") != "" {
		t.Error("reloads although the Theme did not change")
	}
	cfg, _ := a.settings.Load()
	if cfg.ViewMode != settings.Unified || cfg.Diff.CollapseThreshold != 250 || cfg.Port != 9000 || cfg.Keys["refresh"] != "shift+r" {
		t.Errorf("saved %+v", cfg)
	}
}

func TestChangingTheThemeReloadsThePage(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, "/settings", settingsForm(func(v url.Values) { v.Set("theme", "gruvbox-light") }))
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Errorf("HX-Refresh %q, status %d: %s", rec.Header().Get("HX-Refresh"), rec.Code, rec.Body.String())
	}
}

func TestInvalidSettingsAreShownAndNotSaved(t *testing.T) {
	a := newApp(t)
	for name, change := range map[string]func(url.Values){
		"threshold":    func(v url.Values) { v.Set("collapse_threshold", "0") },
		"not a number": func(v url.Values) { v.Set("port", "seven") },
		"key conflict": func(v url.Values) { v.Set("key.refresh", "s") },
		"bad key":      func(v url.Values) { v.Set("key.help", "ctrl+") },
		"theme":        func(v url.Values) { v.Set("theme", "nope") },
	} {
		rec := a.do(t, http.MethodPost, "/settings", settingsForm(change))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `class="settings-error"`) {
			t.Errorf("%s: status %d, body %s", name, rec.Code, rec.Body.String())
		}
		if cfg, _ := a.settings.Load(); cfg.Port != 7878 || cfg.Diff.CollapseThreshold != 1000 || cfg.Keys["refresh"] != "r" {
			t.Errorf("%s: saved anyway: %+v", name, cfg)
		}
	}
}
