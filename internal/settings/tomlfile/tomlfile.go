// Package tomlfile is the Settings adapter backed by a TOML file, normally
// $XDG_CONFIG_HOME/scrutiny/config.toml.
package tomlfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"

	"github.com/Coombszy/scrutiny/internal/settings"
)

// Store reads and writes Settings at a single path.
type Store struct {
	path string
}

var _ settings.Store = (*Store)(nil)

// New returns a Store for the TOML file at path.
func New(path string) *Store {
	return &Store{path: path}
}

// Load reads the file over the defaults. A missing file yields the defaults.
func (s *Store) Load() (settings.Settings, error) {
	cfg := settings.Defaults()
	b, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	cfg.Keys = nil // decoded on its own so a partial [keys] keeps the other defaults
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return settings.Defaults(), fmt.Errorf("parse %s: %w", s.path, err)
	}
	cfg = cfg.WithKeyDefaults()
	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", s.path, err)
	}
	return cfg, nil
}

// Save writes cfg atomically, creating parent directories as needed.
func (s *Store) Save(cfg settings.Settings) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
