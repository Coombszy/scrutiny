// Package settings is the Settings port: user settings whose source of truth
// is the config file.
package settings

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// ViewMode is whether a diff is shown Split (side-by-side) or Unified.
type ViewMode string

const (
	Split   ViewMode = "split"
	Unified ViewMode = "unified"
)

// Settings are the user's settings.
type Settings struct {
	Port     int      `toml:"port"`
	Theme    string   `toml:"theme"`
	ViewMode ViewMode `toml:"view_mode"`
	Diff     Diff     `toml:"diff"`
	// Keys binds actions to key sequences: keys separated by spaces, each
	// optionally with modifiers, e.g. "ctrl+k" or "] c".
	Keys map[string]string `toml:"keys"`
}

// DefaultKeys are the hotkeys before any are rebound.
func DefaultKeys() map[string]string {
	return map[string]string{
		"symbol_search":    "ctrl+k",
		"command_palette":  "ctrl+j",
		"next_comment":     "] c",
		"prev_comment":     "[ c",
		"current_comment":  "g c",
		"toggle_view_mode": "s",
		"refresh":          "r",
		"help":             "?",
	}
}

// keyPattern is one key of a binding: modifiers, then a single character
// or a named key.
var keyPattern = regexp.MustCompile(`^((ctrl|alt|shift|meta)\+)*([^\s+]|\+|[a-z][a-z0-9]+)$`)

// ValidBinding reports whether binding is a well-formed key sequence.
func ValidBinding(binding string) bool {
	keys := strings.Fields(binding)
	if len(keys) == 0 || len(keys) > 3 {
		return false
	}
	for _, k := range keys {
		if !keyPattern.MatchString(k) {
			return false
		}
	}
	return true
}

// Diff holds settings for rendering diffs.
type Diff struct {
	// CollapseThreshold is the changed-line count above which a file's diff
	// starts collapsed.
	CollapseThreshold int `toml:"collapse_threshold"`
	// ExpandLines is how many unchanged lines each click on a gap between
	// changes reveals.
	ExpandLines int `toml:"expand_lines"`
}

// Defaults returns the settings used for anything the config file leaves unset.
func Defaults() Settings {
	return Settings{
		Port:     7878,
		Theme:    "gruvbox-dark",
		ViewMode: Split,
		Diff:     Diff{CollapseThreshold: 1000, ExpandLines: 64},
		Keys:     DefaultKeys(),
	}
}

// Validate reports the first invalid field.
func (s Settings) Validate() error {
	switch {
	case s.ViewMode != Split && s.ViewMode != Unified:
		return fmt.Errorf("view_mode must be %q or %q, got %q", Split, Unified, s.ViewMode)
	case s.Port < 1 || s.Port > 65535:
		return fmt.Errorf("port must be between 1 and 65535, got %d", s.Port)
	case s.Diff.CollapseThreshold < 1:
		return fmt.Errorf("diff.collapse_threshold must be positive, got %d", s.Diff.CollapseThreshold)
	case s.Diff.ExpandLines < 1:
		return fmt.Errorf("diff.expand_lines must be positive, got %d", s.Diff.ExpandLines)
	}
	if err := keyConflicts(s.Keys); err != nil {
		return err
	}
	defaults := DefaultKeys()
	for action, binding := range s.Keys {
		if _, ok := defaults[action]; !ok {
			return fmt.Errorf("keys: unknown action %q", action)
		}
		if !ValidBinding(binding) {
			return fmt.Errorf("keys.%s: %q is not a key sequence", action, binding)
		}
	}
	return nil
}

// WithKeyDefaults fills in any action Keys leaves unbound.
func (s Settings) WithKeyDefaults() Settings {
	keys := DefaultKeys()
	maps.Copy(keys, s.Keys)
	s.Keys = keys
	return s
}

// Store loads and saves Settings.
type Store interface {
	Load() (Settings, error)
	Save(Settings) error
}

// keyConflicts finds two actions bound to the same keys, or one bound to a
// prefix of another's sequence (which would always fire first).
func keyConflicts(keys map[string]string) error {
	actions := slices.Sorted(maps.Keys(keys))
	for i, a := range actions {
		for _, b := range actions[i+1:] {
			sa, sb := strings.Join(strings.Fields(keys[a]), " "), strings.Join(strings.Fields(keys[b]), " ")
			if sa == sb || strings.HasPrefix(sb, sa+" ") || strings.HasPrefix(sa, sb+" ") {
				return fmt.Errorf("keys: %s (%q) and %s (%q) conflict", a, keys[a], b, keys[b])
			}
		}
	}
	return nil
}
