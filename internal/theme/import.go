package theme

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ID turns a Theme's name into its ID, the name of its file: "Tokyo Night
// Storm" becomes "tokyo-night-storm".
func ID(name string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Import makes a Theme called name from a pasted colour scheme: a Scrutiny
// Theme in TOML, or a Vim or Neovim one. Vim schemes are read from their
// `highlight` commands (or the output of `:highlight`), Neovim ones from
// `nvim_set_hl` calls or tables of groups; colours computed at runtime are
// not seen, but `local red = "#..."` style palettes are. Values the scheme
// does not give are derived from the ones it does.
func Import(name, src string) (Theme, error) {
	t := Theme{ID: ID(name), Name: strings.TrimSpace(name)}
	if t.ID == "" {
		return Theme{}, errors.New("the name needs a letter or digit")
	}
	if strings.TrimSpace(src) == "" {
		return Theme{}, errors.New("paste a colour scheme")
	}
	var own Theme
	if err := toml.Unmarshal([]byte(src), &own); err == nil && len(own.UI)+len(own.Syntax) > 0 {
		t.Scheme, t.UI, t.Syntax = own.Scheme, own.UI, own.Syntax
		return t, t.validate()
	}
	ui, syntax, err := fromVim(parseVim(darkBranch(src)))
	if err != nil {
		ui, syntax, err = fromVim(parseVim(src)) // there is only a light variant
	}
	if err != nil {
		return Theme{}, err
	}
	t.UI, t.Syntax = ui, syntax
	t.Scheme = "dark"
	if luminance(ui["bg0"]) > 0.5 {
		t.Scheme = "light"
	}
	return t, t.validate()
}

// Create writes t to dir as a new Theme file, refusing to replace one.
func Create(dir string, t Theme) error {
	if err := t.validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(File(dir, t.ID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("there is already a theme %q", t.ID)
	} else if err != nil {
		return err
	}
	b, _ := toml.Marshal(t)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ErrNotFound is returned when editing a Theme that has no file.
var ErrNotFound = errors.New("theme not found")

// Update replaces t's file in dir, which must exist.
func Update(dir string, t Theme) error {
	if err := t.validate(); err != nil {
		return err
	}
	path := File(dir, t.ID)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("%w: %q", ErrNotFound, t.ID)
	}
	tmp, err := os.CreateTemp(dir, ".theme-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	b, _ := toml.Marshal(t)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Delete removes the Theme id's file from dir.
func Delete(dir, id string) error {
	err := os.Remove(File(dir, id))
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	return err
}

// TOML is t as a Theme file.
func (t Theme) TOML() string {
	b, _ := toml.Marshal(t)
	return string(b)
}

// File is the path of the Theme id's file in dir.
func File(dir, id string) string { return filepath.Join(dir, id+".toml") }

var (
	hex3       = regexp.MustCompile(`^#[0-9a-f]{3}$`)
	hex8       = regexp.MustCompile(`^#[0-9a-f]{8}$`)
	rgbPattern = regexp.MustCompile(`^rgba?\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*(?:,\s*([0-9.]+)\s*)?\)$`)
)

// Swatch splits a colour into what a colour picker shows, #rrggbb, and its
// alpha; ok is false for colours it cannot (hsl()).
func Swatch(v string) (hex string, alpha float64, ok bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch {
	case hexPattern.MatchString(v):
		return v, 1, true
	case hex3.MatchString(v):
		return "#" + strings.Repeat(v[1:2], 2) + strings.Repeat(v[2:3], 2) + strings.Repeat(v[3:4], 2), 1, true
	case hex8.MatchString(v):
		a, _ := strconv.ParseUint(v[7:], 16, 8)
		return v[:7], math.Round(float64(a)/255*100) / 100, true
	}
	m := rgbPattern.FindStringSubmatch(v)
	if m == nil {
		return "", 0, false
	}
	alpha = 1
	if m[4] != "" {
		alpha, _ = strconv.ParseFloat(m[4], 64)
	}
	part := func(s string) int { n, _ := strconv.Atoi(s); return min(n, 255) }
	return fmt.Sprintf("#%02x%02x%02x", part(m[1]), part(m[2]), part(m[3])), alpha, true
}

// WithAlpha is hex at alpha, as rgba() when it is see-through.
func WithAlpha(hex string, alpha float64) string {
	if alpha >= 1 || !hexPattern.MatchString(hex) {
		return hex
	}
	return rgba(hex, alpha)
}

// Keys are the UI and syntax keys every Theme has.
func Keys() (ui, syntax []string) {
	d := builtinDefault()
	return slices.Sorted(maps.Keys(d.UI)), slices.Sorted(maps.Keys(d.Syntax))
}

// vimScheme is what a pasted Vim or Neovim colour scheme defines.
type vimScheme struct {
	groups   map[string]vimGroup
	terminal map[int]string // g:terminal_color_N
}

type vimGroup struct{ fg, bg, link string }

var (
	hexPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// hi[ghlight][!] [default] link From To
	vimLink = regexp.MustCompile(`^hi(?:ghlight)?!?\s+(?:def(?:ault)?\s+)?link\s+(\S+)\s+(\S+)`)
	// hi[ghlight][!] [default] Group guifg=... guibg=...
	vimHi = regexp.MustCompile(`^hi(?:ghlight)?!?\s+(?:def(?:ault)?\s+)?(\S+)\s+(.*)`)
	// The output of :highlight: "Group xxx guifg=..." or "Group xxx links to To".
	vimOutLink = regexp.MustCompile(`^(\S+)\s+xxx\s+links to\s+(\S+)`)
	vimOut     = regexp.MustCompile(`^(\S+)\s+xxx\s+(.*)`)
	vimAttr    = regexp.MustCompile(`\b(guifg|guibg)=['"]?([^\s'"]+)`)
	terminal   = regexp.MustCompile(`terminal_color_(\d+)\s*=\s*["'](#[0-9a-fA-F]{6})["']`)
	ansiList   = regexp.MustCompile(`terminal_ansi_colors\s*=\s*\[([^\]]*)\]`)
	quotedHex  = regexp.MustCompile(`["'](#[0-9a-fA-F]{6})["']`)
	// local red = "#fb4934", or red = "#fb4934" in a palette table.
	luaVar = regexp.MustCompile(`\b([A-Za-z_]\w*)\s*=\s*["'](#[0-9a-fA-F]{6})["']`)
	// nvim_set_hl(0, "Group", { ... }), Group = { ... } or ["@group"] = { ... }.
	luaGroup = regexp.MustCompile(`(?:\[\s*)?["']?(@?[A-Za-z][\w.@-]*)["']?(?:\s*\])?\s*[,=]\s*\{([^{}]*)\}`)
	luaField = regexp.MustCompile(`\b(fg|bg|foreground|background|link)\s*=\s*(?:["']([^"']*)["']|([A-Za-z_][\w.]*))`)
)

func parseVim(src string) vimScheme {
	s := vimScheme{groups: map[string]vimGroup{}, terminal: map[int]string{}}
	set := func(name string, change func(*vimGroup)) {
		g := s.groups[name]
		change(&g)
		s.groups[name] = g
	}
	for line := range strings.Lines(src) {
		line = strings.TrimLeft(strings.TrimSpace(line), ":")
		if m := firstMatch(line, vimLink, vimOutLink); m != nil {
			set(m[1], func(g *vimGroup) { *g = vimGroup{link: m[2]} })
		} else if m := firstMatch(line, vimHi, vimOut); m != nil && m[1] != "clear" {
			for _, a := range vimAttr.FindAllStringSubmatch(m[2], -1) {
				set(m[1], func(g *vimGroup) {
					if a[1] == "guifg" {
						g.fg = a[2]
					} else {
						g.bg = a[2]
					}
				})
			}
		}
	}
	if m := ansiList.FindStringSubmatch(src); m != nil {
		for i, c := range quotedHex.FindAllStringSubmatch(m[1], -1) {
			s.terminal[i] = c[1]
		}
	}
	for _, m := range terminal.FindAllStringSubmatch(src, -1) {
		n, _ := strconv.Atoi(m[1])
		s.terminal[n] = m[2]
	}
	// The first definition is the palette's: later ones are groups' fields.
	vars := map[string]string{}
	for _, m := range luaVar.FindAllStringSubmatch(src, -1) {
		if _, ok := vars[m[1]]; !ok {
			vars[m[1]] = m[2]
		}
	}
	for _, m := range luaGroup.FindAllStringSubmatch(src, -1) {
		var g vimGroup
		for _, f := range luaField.FindAllStringSubmatch(m[2], -1) {
			v := f[2]
			if f[3] != "" { // a variable such as colors.red
				v = vars[f[3][strings.LastIndex(f[3], ".")+1:]]
			}
			switch f[1] {
			case "fg", "foreground":
				g.fg = v
			case "bg", "background":
				g.bg = v
			case "link":
				g.link = v
			}
		}
		if g != (vimGroup{}) {
			s.groups[m[1]] = g
		}
	}
	return s
}

var (
	vimIf         = regexp.MustCompile(`^if\b`)
	vimBackground = regexp.MustCompile(`&(?:background|bg)\s*==[#?]?\s*['"](dark|light)['"]`)
	vimElse       = regexp.MustCompile(`^el(?:se?)?$|^else\b`)
	vimEndif      = regexp.MustCompile(`^en(?:d(?:if?)?)?$|^endif\b`)
)

// darkBranch drops the lines a Vim colour scheme runs for a light
// background, so a scheme with both variants gives its dark one.
func darkBranch(src string) string {
	type frame struct{ background, skip bool }
	var stack []frame
	skipping := func() bool {
		return slices.ContainsFunc(stack, func(f frame) bool { return f.skip })
	}
	var b strings.Builder
	for line := range strings.Lines(src) {
		trimmed := strings.TrimSpace(line)
		switch {
		case vimIf.MatchString(trimmed):
			m := vimBackground.FindStringSubmatch(trimmed)
			stack = append(stack, frame{background: m != nil, skip: m != nil && m[1] == "light"})
			continue
		case vimElse.MatchString(trimmed) && len(stack) > 0:
			if f := &stack[len(stack)-1]; f.background {
				f.skip = !f.skip
			}
			continue
		case vimEndif.MatchString(trimmed) && len(stack) > 0:
			stack = stack[:len(stack)-1]
			continue
		}
		if !skipping() {
			b.WriteString(line)
		}
	}
	return b.String()
}

func firstMatch(s string, patterns ...*regexp.Regexp) []string {
	for _, p := range patterns {
		if m := p.FindStringSubmatch(s); m != nil {
			return m
		}
	}
	return nil
}

// colour is the first of groups' fg (or bg) colours, following links.
func (s vimScheme) colour(bg bool, groups ...string) string {
	for _, name := range groups {
		for range 10 {
			g, ok := s.groups[name]
			if !ok {
				break
			}
			if g.link != "" {
				name = g.link
				continue
			}
			v := g.fg
			if bg {
				v = g.bg
			}
			if hexPattern.MatchString(v) {
				return strings.ToLower(v)
			}
			break
		}
	}
	return ""
}

// syntaxGroups are, for each capture name, the Vim groups to take its
// colour from after Neovim's own "@capture" group.
var syntaxGroups = map[string][]string{
	"keyword":             {"Keyword", "Statement"},
	"label":               {"Label", "Statement"},
	"string":              {"String"},
	"text.literal":        {"@markup.raw", "String"},
	"embedded":            {"@markup.raw", "String"},
	"string.special.key":  {"@property", "Identifier"},
	"property":            {"@variable.member", "@field", "Identifier"},
	"attribute":           {"PreProc"},
	"escape":              {"@string.escape", "SpecialChar", "Special"},
	"string.escape":       {"SpecialChar", "Special"},
	"comment":             {"Comment"},
	"function":            {"Function"},
	"function.method":     {"@method", "@function", "Function"},
	"function.builtin":    {"Special", "Function"},
	"type":                {"Type"},
	"number":              {"Number", "Constant"},
	"boolean":             {"Boolean", "Constant"},
	"constant":            {"Constant"},
	"operator":            {"Operator", "Statement"},
	"punctuation":         {"@punctuation.delimiter", "Delimiter"},
	"punctuation.special": {"Special"},
	"variable":            {},
	"text.title":          {"@markup.heading", "@text.title", "Title"},
	"text.uri":            {"@markup.link.url", "@text.uri", "Underlined"},
	"text.reference":      {"@markup.link", "@text.reference", "Identifier"},
}

func fromVim(s vimScheme) (ui, syntax map[string]string, err error) {
	fg, bg := s.colour(false, "Normal"), s.colour(true, "Normal")
	if fg == "" || bg == "" {
		return nil, nil, errors.New("no Normal colours found; paste a Vim or Neovim colour scheme, or a Scrutiny theme")
	}
	def := builtinDefault().UI
	// accent prefers the terminal colour, then the groups, then Gruvbox's,
	// passing over any too close to the background to read (such as the fg
	// of a reversed ErrorMsg).
	accent := func(key string, term int, groups ...string) string {
		for _, c := range append([]string{strings.ToLower(s.terminal[term])}, mapSlice(groups, func(g string) string { return s.colour(false, g) })...) {
			if c != "" && contrast(c, bg) >= 1.5 {
				return c
			}
		}
		return def[key]
	}
	or := func(c, fallback string) string {
		if c == "" {
			return fallback
		}
		return c
	}
	ui = map[string]string{
		"bg0":    bg,
		"fg":     fg,
		"fg-dim": mix(fg, bg, 0.25),
		"gray":   or(s.colour(false, "Comment"), mix(fg, bg, 0.5)),
		"red":    accent("red", 1, "DiagnosticError", "ErrorMsg"),
		"green":  accent("green", 2, "DiagnosticOk", "String"),
		"yellow": accent("yellow", 3, "DiagnosticWarn", "WarningMsg", "Type"),
		"blue":   accent("blue", 4, "DiagnosticInfo", "Function"),
		"purple": accent("purple", 5, "Constant", "Number"),
		"aqua":   accent("aqua", 6, "DiagnosticHint", "Special"),
	}
	ui["orange"] = mix(ui["red"], ui["yellow"], 0.5)
	ui["bg1"] = or(s.colour(true, "CursorLine", "ColorColumn"), mix(bg, fg, 0.1))
	if ui["bg1"] == bg {
		ui["bg1"] = mix(bg, fg, 0.1)
	}
	ui["bg2"] = mix(bg, fg, 0.2)
	ui["bg-soft"] = mix(bg, ui["bg1"], 0.5)
	ui["add-bg"], ui["add-ln"] = rgba(ui["green"], 0.14), rgba(ui["green"], 0.24)
	ui["del-bg"], ui["del-ln"] = rgba(ui["red"], 0.14), rgba(ui["red"], 0.24)

	syntax = map[string]string{}
	for _, k := range slices.Sorted(maps.Keys(builtinDefault().Syntax)) {
		syntax[k] = s.colour(false, append([]string{"@" + k}, syntaxGroups[k]...)...)
	}
	syntax["variable"] = or(syntax["variable"], fg)
	syntax["punctuation"] = or(syntax["punctuation"], ui["fg-dim"])
	for k, v := range syntax {
		syntax[k] = or(v, fg)
	}
	return ui, syntax, nil
}

func rgb(hex string) (r, g, b float64) {
	n, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return float64(n >> 16 & 0xff), float64(n >> 8 & 0xff), float64(n & 0xff)
}

// mix is a weight of the way from a to b.
func mix(a, b string, weight float64) string {
	ar, ag, ab := rgb(a)
	br, bg, bb := rgb(b)
	at := func(x, y float64) int { return int(math.Round(x + (y-x)*weight)) }
	return fmt.Sprintf("#%02x%02x%02x", at(ar, br), at(ag, bg), at(ab, bb))
}

func rgba(hex string, alpha float64) string {
	r, g, b := rgb(hex)
	return fmt.Sprintf("rgba(%d, %d, %d, %.2f)", int(r), int(g), int(b), alpha)
}

func mapSlice(in []string, f func(string) string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// contrast is the WCAG contrast ratio of a and b, 1 to 21.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	return (max(la, lb) + 0.05) / (min(la, lb) + 0.05)
}

// luminance is hex's relative luminance, 0 (black) to 1 (white).
func luminance(hex string) float64 {
	lin := func(c float64) float64 {
		c /= 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	r, g, b := rgb(hex)
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}
