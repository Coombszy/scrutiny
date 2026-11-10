package theme_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/theme"
)

func TestBuiltinThemes(t *testing.T) {
	themes, errs := theme.Load("")
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	if len(themes) != 2 || themes[0].ID != "gruvbox-dark" || themes[1].ID != "gruvbox-light" || themes[1].Name != "Gruvbox Light" {
		t.Errorf("themes = %+v", themes)
	}
}

func TestCSSCoversChromeAndSyntax(t *testing.T) {
	themes, _ := theme.Load("")
	light, _ := theme.Find(themes, "gruvbox-light")
	css := light.CSS()
	for _, want := range []string{
		"color-scheme: light", "--bg0: #fbf1c7;", "--add-bg: rgba(121, 116, 14, 0.14);",
		".hl-keyword { color: #9d0006; }", ".hl-function-method { color: #427b58; }",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS missing %q", want)
		}
	}
}

func TestDropInThemesOverrideAndFallBack(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "solar.toml"), `name = "Solar"
scheme = "light"
[ui]
bg0 = "#fdf6e3"
[syntax]
keyword = "#859900"
`)
	write(t, filepath.Join(dir, "gruvbox-dark.toml"), "name = \"My Gruvbox\"\n")
	write(t, filepath.Join(dir, "evil.toml"), "name = \"Evil\"\n[ui]\nbg0 = \"red; } body { display: none\"\n")
	write(t, filepath.Join(dir, "notes.txt"), "not a theme")

	themes, errs := theme.Load(dir)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "evil.toml") {
		t.Errorf("errs = %v, want one for evil.toml", errs)
	}
	find := func(id string) (theme.Theme, bool) { return theme.Find(themes, id) }
	solar, ok := find("solar")
	if !ok || solar.Name != "Solar" {
		t.Fatalf("solar = %+v, %v", solar, ok)
	}
	css := solar.CSS()
	if !strings.Contains(css, "--bg0: #fdf6e3;") || !strings.Contains(css, "--fg: #ebdbb2;") || !strings.Contains(css, ".hl-keyword { color: #859900; }") {
		t.Errorf("solar CSS should use its own colours and fall back to Gruvbox Dark:\n%s", css)
	}
	if mine, _ := find("gruvbox-dark"); mine.Name != "My Gruvbox" {
		t.Errorf("drop-in gruvbox-dark did not override: %q", mine.Name)
	}
	if _, ok := find("evil"); ok {
		t.Error("invalid theme was loaded")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
