// Package theme loads Themes: named colours for the interface and for
// syntax highlighting. Gruvbox Dark and Gruvbox Light are built in; more
// can be dropped into $XDG_CONFIG_HOME/scrutiny/themes as TOML files.
package theme

import (
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed builtin/*.toml
var builtins embed.FS

// Default is the Theme used when the configured one is unknown.
const Default = "gruvbox-dark"

// Theme is a named set of colours. UI keys are the interface's CSS custom
// properties (e.g. "bg0", "add-bg"); Syntax keys are highlight capture
// names (e.g. "keyword", "function.method").
type Theme struct {
	ID     string            `toml:"-"` // the file name without .toml
	Custom bool              `toml:"-"` // its file is in the themes directory, so it can be edited
	Name   string            `toml:"name"`
	Scheme string            `toml:"scheme"` // "dark" or "light"
	UI     map[string]string `toml:"ui"`
	Syntax map[string]string `toml:"syntax"`
}

var (
	keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
	// colourPattern admits hex colours and rgb()/rgba()/hsl()/hsla() with
	// plain numeric arguments, so values cannot break out of CSS.
	colourPattern = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\([0-9.,% ]+\))$`)
)

// Load returns the built-in Themes overlaid with those in dir, sorted by
// ID, and an error for each file in dir that is not a valid Theme. Values a
// Theme leaves out fall back to Gruvbox Dark's.
func Load(dir string) ([]Theme, []error) {
	byID := map[string]Theme{}
	var errs []error
	read := func(fsys fs.FS, name string, custom bool) {
		id := strings.TrimSuffix(filepath.Base(name), ".toml")
		b, err := fs.ReadFile(fsys, name)
		if err == nil {
			var t Theme
			if err = toml.Unmarshal(b, &t); err == nil {
				t.ID, t.Custom = id, custom
				err = t.validate()
			}
			if err == nil {
				byID[id] = t
				return
			}
		}
		errs = append(errs, fmt.Errorf("theme %s: %w", name, err))
	}
	entries, _ := fs.Glob(builtins, "builtin/*.toml")
	for _, e := range entries {
		read(builtins, e, false)
	}
	if dir != "" {
		files, _ := filepath.Glob(filepath.Join(dir, "*.toml"))
		for _, f := range files {
			read(os.DirFS(dir), filepath.Base(f), true)
		}
	}
	themes := make([]Theme, 0, len(byID))
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		t := byID[id]
		t.UI = merge(builtinDefault().UI, t.UI)
		t.Syntax = merge(builtinDefault().Syntax, t.Syntax)
		if t.Name == "" {
			t.Name = id
		}
		if t.Scheme != "light" {
			t.Scheme = "dark"
		}
		themes = append(themes, t)
	}
	return themes, errs
}

// builtinDefault is the shipped Gruvbox Dark, the fallback for every value.
func builtinDefault() Theme {
	b, _ := builtins.ReadFile("builtin/" + Default + ".toml")
	var t Theme
	_ = toml.Unmarshal(b, &t)
	return t
}

func merge(base, over map[string]string) map[string]string {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]string{}
	}
	maps.Copy(out, over)
	return out
}

func (t Theme) validate() error {
	for section, m := range map[string]map[string]string{"ui": t.UI, "syntax": t.Syntax} {
		for k, v := range m {
			if !keyPattern.MatchString(k) {
				return fmt.Errorf("[%s] key %q is not allowed", section, k)
			}
			if !colourPattern.MatchString(strings.TrimSpace(v)) {
				return fmt.Errorf("[%s] %s = %q is not a colour", section, k, v)
			}
		}
	}
	return nil
}

// Find returns the Theme with id.
func Find(themes []Theme, id string) (Theme, bool) {
	i := slices.IndexFunc(themes, func(t Theme) bool { return t.ID == id })
	if i < 0 {
		return Theme{}, false
	}
	return themes[i], true
}

// CSS renders the Theme as custom properties and highlight rules.
func (t Theme) CSS() string {
	var b strings.Builder
	fmt.Fprintf(&b, ":root {\n  color-scheme: %s;\n", t.Scheme)
	for _, k := range slices.Sorted(maps.Keys(t.UI)) {
		fmt.Fprintf(&b, "  --%s: %s;\n", k, strings.TrimSpace(t.UI[k]))
	}
	b.WriteString("}\n")
	for _, k := range slices.Sorted(maps.Keys(t.Syntax)) {
		colour := strings.TrimSpace(t.Syntax[k])
		fmt.Fprintf(&b, ".hl-%s { color: %s; }\n", strings.ReplaceAll(k, ".", "-"), colour)
		if cs := githubClasses[k]; len(cs) > 0 {
			fmt.Fprintf(&b, ".markdown .%s { color: %s; }\n", strings.Join(cs, ", .markdown ."), colour)
		}
	}
	return b.String()
}

// githubClasses are the classes GitHub puts on code it highlights in
// comment bodies, by the capture name whose colour they take. pl-s1 and
// pl-smi stay plain, as on GitHub.
var githubClasses = map[string][]string{
	"keyword":       {"pl-k"},
	"string":        {"pl-s", "pl-pds", "pl-sr"},
	"string.escape": {"pl-cce", "pl-pse"},
	"comment":       {"pl-c"},
	"constant":      {"pl-c1"},
	"function":      {"pl-en"},
	"variable":      {"pl-v"},
	"attribute":     {"pl-e"},
	"type":          {"pl-ent"},
	"punctuation":   {"pl-kos"},
	"text.title":    {"pl-mh"},
}
