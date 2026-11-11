package web_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

func TestSettingsPageOffersANewThemeAndEditsCustomOnes(t *testing.T) {
	a := newApp(t)
	body := a.do(t, http.MethodGet, "/settings", nil).Body.String()
	// The buttons are in the Settings form, so they must not inherit its
	// outerHTML swap, which would replace the dialog.
	mustContain(t, body, `hx-get="/settings/themes/new" hx-target="#theme-editor" hx-swap="innerHTML"`, `id="theme-editor"`)
	if strings.Contains(body, `/edit"`) {
		t.Error("offers to edit a built-in theme")
	}
	os.WriteFile(filepath.Join(a.themesDir, "paper.toml"), []byte("name = \"Paper\"\n[ui]\nbg0 = \"#f8f8f8\"\n"), 0o644)
	a.do(t, http.MethodPost, "/settings/theme", url.Values{"theme": {"paper"}})
	mustContain(t, a.do(t, http.MethodGet, "/settings", nil).Body.String(), `hx-get="/settings/themes/paper/edit" hx-target="#theme-editor" hx-swap="innerHTML"`)
}

func TestTheThemeEditorStartsFromTheCurrentTheme(t *testing.T) {
	a := newApp(t)
	body := a.do(t, http.MethodGet, "/settings/themes/new", nil).Body.String()
	mustContain(t, body, `hx-post="/settings/themes"`, `name="mode" value="paste" checked`, a.themesDir,
		`type="color" name="ui.bg0" value="#282828"`,
		`name="ui.add-bg" value="#b8bb26"`, `name="alpha.ui.add-bg" value="0.14"`,
		`name="syntax.keyword" value="#fb4934"`)
}

func TestCreatingAThemeFromAPasteSavesItAndSwitchesToIt(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, "/settings/themes", url.Values{
		"name":   {"Tokyo Night"},
		"mode":   {"paste"},
		"source": {"hi Normal guifg=#c0caf5 guibg=#1a1b26\nhi Keyword guifg=#bb9af7\n"},
	})
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("HX-Refresh %q, status %d: %s", rec.Header().Get("HX-Refresh"), rec.Code, rec.Body.String())
	}
	if cfg, _ := a.settings.Load(); cfg.Theme != "tokyo-night" {
		t.Errorf("theme = %q", cfg.Theme)
	}
	if _, err := os.Stat(filepath.Join(a.themesDir, "tokyo-night.toml")); err != nil {
		t.Error(err)
	}
	mustContain(t, a.do(t, http.MethodGet, "/theme.css", nil).Body.String(), "--bg0: #1a1b26;", ".hl-keyword { color: #bb9af7; }")
}

func TestCreatingAThemeFromThePickers(t *testing.T) {
	a := newApp(t)
	rec := a.do(t, http.MethodPost, "/settings/themes", url.Values{
		"name": {"Picked"}, "mode": {"pick"}, "scheme": {"light"},
		"ui.bg0": {"#ffffff"}, "ui.add-bg": {"#00ff00"}, "alpha.ui.add-bg": {"0.2"},
		"syntax.keyword": {"#aa0000"},
	})
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	mustContain(t, a.do(t, http.MethodGet, "/theme.css", nil).Body.String(),
		"color-scheme: light", "--bg0: #ffffff;", "--add-bg: rgba(0, 255, 0, 0.20);", ".hl-keyword { color: #aa0000; }")
}

func TestEditingAThemeRewritesItsFile(t *testing.T) {
	a := newApp(t)
	path := filepath.Join(a.themesDir, "paper.toml")
	os.WriteFile(path, []byte("# mine\nname = \"Paper\"\nscheme = \"light\"\n[ui]\nbg0 = \"#f8f8f8\"\n"), 0o644)

	body := a.do(t, http.MethodGet, "/settings/themes/paper/edit", nil).Body.String()
	mustContain(t, body, `hx-post="/settings/themes/paper"`, `value="Paper"`, `name="mode" value="pick" checked`,
		`name="ui.bg0" value="#f8f8f8"`, "# mine", `<option value="light" selected`)

	rec := a.do(t, http.MethodPost, "/settings/themes/paper", url.Values{
		"name": {"Paper Renamed"}, "mode": {"pick"}, "scheme": {"light"}, "ui.bg0": {"#eeeeee"},
	})
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	b, _ := os.ReadFile(path)
	mustContain(t, string(b), "Paper Renamed", "#eeeeee")
	if cfg, _ := a.settings.Load(); cfg.Theme != "paper" {
		t.Errorf("theme = %q", cfg.Theme)
	}
}

func TestBuiltinThemesCannotBeEdited(t *testing.T) {
	a := newApp(t)
	if rec := a.do(t, http.MethodGet, "/settings/themes/gruvbox-dark/edit", nil); rec.Code != http.StatusNotFound {
		t.Errorf("edit form: status %d", rec.Code)
	}
	if rec := a.do(t, http.MethodPost, "/settings/themes/gruvbox-dark", url.Values{"name": {"X"}, "mode": {"pick"}}); rec.Code != http.StatusNotFound {
		t.Errorf("save: status %d", rec.Code)
	}
}

func TestThemesThatCannotBeSavedAreExplained(t *testing.T) {
	a := newApp(t)
	for name, form := range map[string]url.Values{
		"builtin":    {"name": {"Gruvbox Dark"}, "mode": {"paste"}, "source": {"hi Normal guifg=#000000 guibg=#ffffff"}},
		"no colour":  {"name": {"Blank"}, "mode": {"paste"}, "source": {"hi Comment guifg=#888888"}},
		"bad colour": {"name": {"Bad"}, "mode": {"pick"}, "ui.bg0": {"red; }"}},
		"no name":    {"name": {" "}, "mode": {"pick"}},
	} {
		rec := a.do(t, http.MethodPost, "/settings/themes", form)
		if rec.Code != http.StatusOK || rec.Header().Get("HX-Refresh") != "" || !strings.Contains(rec.Body.String(), `class="settings-error"`) {
			t.Errorf("%s: status %d, body %s", name, rec.Code, rec.Body.String())
		}
	}
	if cfg, _ := a.settings.Load(); cfg.Theme != "gruvbox-dark" {
		t.Errorf("theme changed to %q", cfg.Theme)
	}
}

func TestTheThemeEditorCopiesFromAnExistingTheme(t *testing.T) {
	a := newApp(t)
	body := a.do(t, http.MethodGet, "/settings/themes/new?from=gruvbox-light", nil).Body.String()
	mustContain(t, body, `value="Gruvbox Light copy"`, `<option value="gruvbox-light" selected`,
		`name="mode" value="pick" checked`, `name="ui.bg0" value="#fbf1c7"`, `<option value="light" selected`,
		`hx-get="/settings/themes/new" hx-include="#theme-editor-name"`, `[ui]`)
	body = a.do(t, http.MethodGet, "/settings/themes/new?from=gruvbox-light&name=Mine", nil).Body.String()
	mustContain(t, body, `value="Mine"`)

	os.WriteFile(filepath.Join(a.themesDir, "paper.toml"), []byte("name = \"Paper\"\n[ui]\nbg0 = \"#f8f8f8\"\n"), 0o644)
	body = a.do(t, http.MethodGet, "/settings/themes/paper/edit?from=gruvbox-light&name=Paper", nil).Body.String()
	mustContain(t, body, `hx-post="/settings/themes/paper"`, `value="Paper"`, `name="ui.bg0" value="#fbf1c7"`)
}

func TestDeletingAThemeRemovesItsFileAndFallsBackToTheDefault(t *testing.T) {
	a := newApp(t)
	path := filepath.Join(a.themesDir, "paper.toml")
	os.WriteFile(path, []byte("name = \"Paper\"\n[ui]\nbg0 = \"#f8f8f8\"\n"), 0o644)
	a.do(t, http.MethodPost, "/settings/theme", url.Values{"theme": {"paper"}})
	mustContain(t, a.do(t, http.MethodGet, "/settings", nil).Body.String(),
		`hx-post="/settings/themes/paper/delete"`, `hx-confirm="Delete the theme Paper?`)

	rec := a.do(t, http.MethodPost, "/settings/themes/paper/delete", url.Values{})
	if rec.Header().Get("HX-Refresh") != "true" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	if cfg, _ := a.settings.Load(); cfg.Theme != "gruvbox-dark" {
		t.Errorf("theme = %q", cfg.Theme)
	}
	if rec := a.do(t, http.MethodPost, "/settings/themes/gruvbox-dark/delete", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("deleting a built-in theme: status %d", rec.Code)
	}
}
