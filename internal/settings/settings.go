// Package settings is the Settings port: user settings whose source of truth
// is the config file.
package settings

import "fmt"

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
}

// Diff holds settings for rendering diffs.
type Diff struct {
	// CollapseThreshold is the changed-line count above which a file's diff
	// starts collapsed.
	CollapseThreshold int `toml:"collapse_threshold"`
}

// Defaults returns the settings used for anything the config file leaves unset.
func Defaults() Settings {
	return Settings{
		Port:     7878,
		Theme:    "gruvbox-dark",
		ViewMode: Split,
		Diff:     Diff{CollapseThreshold: 1000},
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
	}
	return nil
}

// Store loads and saves Settings.
type Store interface {
	Load() (Settings, error)
	Save(Settings) error
}
