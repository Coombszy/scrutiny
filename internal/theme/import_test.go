package theme_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/theme"
)

func TestIDIsTheNameAsAFileName(t *testing.T) {
	for name, want := range map[string]string{"Tokyo Night Storm": "tokyo-night-storm", "  Rosé Pine!! ": "ros-pine", "***": ""} {
		if got := theme.ID(name); got != want {
			t.Errorf("ID(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestImportReadsAVimColourScheme(t *testing.T) {
	src := `set background=light
hi clear
let g:colors_name = "paper"
hi Normal guifg=#202020 guibg=#F8F8F8 ctermfg=0
hi Comment guifg=#888888 gui=italic
highlight! Keyword guifg=#aa0000
hi def link Statement Keyword
hi link Function Identifier
hi Identifier guifg='#0055aa'
let g:terminal_color_2 = '#008800'
`
	th, err := theme.Import("Paper", src)
	if err != nil {
		t.Fatal(err)
	}
	if th.ID != "paper" || th.Name != "Paper" || th.Scheme != "light" {
		t.Errorf("theme = %q %q %q", th.ID, th.Name, th.Scheme)
	}
	for k, want := range map[string]string{"bg0": "#f8f8f8", "fg": "#202020", "gray": "#888888", "green": "#008800", "add-bg": "rgba(0, 136, 0, 0.14)"} {
		if th.UI[k] != want {
			t.Errorf("ui %s = %q, want %q", k, th.UI[k], want)
		}
	}
	for k, want := range map[string]string{"keyword": "#aa0000", "label": "#aa0000", "function": "#0055aa", "function.method": "#0055aa", "comment": "#888888", "variable": "#202020"} {
		if th.Syntax[k] != want {
			t.Errorf("syntax %s = %q, want %q", k, th.Syntax[k], want)
		}
	}
}

func TestImportReadsHighlightOutput(t *testing.T) {
	src := `Normal         xxx guifg=#ebdbb2 guibg=#282828
Keyword        xxx guifg=#fb4934
Conditional    xxx links to Keyword
@keyword       xxx links to Keyword`
	th, err := theme.Import("Out", src)
	if err != nil {
		t.Fatal(err)
	}
	if th.Scheme != "dark" || th.Syntax["keyword"] != "#fb4934" {
		t.Errorf("scheme %q, keyword %q", th.Scheme, th.Syntax["keyword"])
	}
}

func TestImportReadsANeovimLuaColourScheme(t *testing.T) {
	src := `local c = { fg = "#c0caf5", bg = "#1a1b26", red = "#f7768e" }
local hl = vim.api.nvim_set_hl
hl(0, "Normal", { fg = c.fg, bg = c.bg })
hl(0, "@keyword", { fg = c.red, italic = true })
hl(0, "@function.method", { link = "Function" })
local groups = {
  Function = { fg = "#7aa2f7" },
  ["@string"] = { fg = '#9ece6a' },
}`
	th, err := theme.Import("Night", src)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"keyword": "#f7768e", "function.method": "#7aa2f7", "string": "#9ece6a"} {
		if th.Syntax[k] != want {
			t.Errorf("syntax %s = %q, want %q", k, th.Syntax[k], want)
		}
	}
	if th.UI["bg0"] != "#1a1b26" || th.UI["fg"] != "#c0caf5" {
		t.Errorf("ui = %v", th.UI)
	}
}

func TestImportTakesAScrutinyTheme(t *testing.T) {
	th, err := theme.Import("Mine", "scheme = \"light\"\n[ui]\nbg0 = \"#ffffff\"\n")
	if err != nil || th.UI["bg0"] != "#ffffff" || th.Scheme != "light" || th.Name != "Mine" {
		t.Errorf("theme = %+v, %v", th, err)
	}
}

func TestImportRejectsWhatItCannotRead(t *testing.T) {
	for name, src := range map[string]string{
		"empty":     " ",
		"no normal": "hi Comment guifg=#888888",
		"bad toml":  "[ui]\nbg0 = \"red; }\"\n",
	} {
		if _, err := theme.Import("X", src); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := theme.Import("!!", "hi Normal guifg=#000000 guibg=#ffffff"); err == nil {
		t.Error("a name without letters or digits was accepted")
	}
}

func TestSavedThemesLoadAndAreNotReplaced(t *testing.T) {
	dir := t.TempDir()
	th, _ := theme.Import("Paper", "hi Normal guifg=#202020 guibg=#f8f8f8")
	if err := theme.Create(dir, th); err != nil {
		t.Fatal(err)
	}
	if err := theme.Create(dir, th); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("second save: %v", err)
	}
	themes, errs := theme.Load(dir)
	got, ok := theme.Find(themes, "paper")
	if len(errs) > 0 || !ok || !got.Custom || got.Name != "Paper" || got.Scheme != "light" || got.UI["bg0"] != "#f8f8f8" {
		t.Errorf("loaded %+v, %v", got, errs)
	}

	got.Name, got.UI["bg0"] = "Paper Two", "#eeeeee"
	if err := theme.Update(dir, got); err != nil {
		t.Fatal(err)
	}
	themes, _ = theme.Load(dir)
	if again, _ := theme.Find(themes, "paper"); again.Name != "Paper Two" || again.UI["bg0"] != "#eeeeee" {
		t.Errorf("updated %+v", again)
	}
	if err := theme.Delete(dir, "paper"); err != nil {
		t.Fatal(err)
	}
	if themes, _ = theme.Load(dir); len(themes) != 2 {
		t.Errorf("after delete: %d themes", len(themes))
	}
	if err := theme.Update(dir, got); !errors.Is(err, theme.ErrNotFound) {
		t.Errorf("updating a missing theme: %v", err)
	}
	if err := theme.Delete(dir, "paper"); !errors.Is(err, theme.ErrNotFound) {
		t.Errorf("deleting a missing theme: %v", err)
	}
}

func TestSwatchSplitsColoursForAPicker(t *testing.T) {
	for v, want := range map[string]struct {
		hex   string
		alpha float64
		ok    bool
	}{
		"#ABCDEF":                  {"#abcdef", 1, true},
		"#abc":                     {"#aabbcc", 1, true},
		"#11223380":                {"#112233", 0.5, true},
		"rgba(184, 187, 38, 0.14)": {"#b8bb26", 0.14, true},
		"rgb(1,2,3)":               {"#010203", 1, true},
		"hsl(10, 50%, 50%)":        {"", 0, false},
	} {
		hex, alpha, ok := theme.Swatch(v)
		if hex != want.hex || alpha != want.alpha || ok != want.ok {
			t.Errorf("Swatch(%q) = %q %v %v", v, hex, alpha, ok)
		}
	}
	if got := theme.WithAlpha("#b8bb26", 0.14); got != "rgba(184, 187, 38, 0.14)" {
		t.Errorf("WithAlpha = %q", got)
	}
	if got := theme.WithAlpha("#b8bb26", 1); got != "#b8bb26" {
		t.Errorf("WithAlpha opaque = %q", got)
	}
}

func TestImportPrefersAVimSchemesDarkVariant(t *testing.T) {
	both := `if &background ==# 'dark'
  hi Normal guifg=#ebdbb2 guibg=#282828
  if has('nvim')
    let g:terminal_color_1 = '#fb4934'
  endif
else
  hi Normal guifg=#3c3836 guibg=#fbf1c7
  let g:terminal_color_1 = '#9d0006'
endif
hi Comment guifg=#928374`
	th, err := theme.Import("Both", both)
	if err != nil || th.Scheme != "dark" || th.UI["bg0"] != "#282828" || th.UI["red"] != "#fb4934" || th.Syntax["comment"] != "#928374" {
		t.Errorf("theme = %v %v, %v", th.Scheme, th.UI, err)
	}
	light, err := theme.Import("Light", "if &background == 'light'\nhi Normal guifg=#3c3836 guibg=#fbf1c7\nendif")
	if err != nil || light.Scheme != "light" {
		t.Errorf("light-only scheme: %v, %v", light.Scheme, err)
	}
}
