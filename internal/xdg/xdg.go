// Package xdg resolves Scrutiny's XDG base directories. The XDG layout is
// used on both Linux and macOS.
package xdg

import (
	"os"
	"path/filepath"
)

const app = "scrutiny"

// Dirs are Scrutiny's own subdirectories of the XDG base directories.
type Dirs struct {
	Config string // $XDG_CONFIG_HOME/scrutiny
	State  string // $XDG_STATE_HOME/scrutiny
	Data   string // $XDG_DATA_HOME/scrutiny
}

// Resolve reads the XDG_* environment variables, falling back to the spec
// defaults under $HOME when they are unset or not absolute.
func Resolve() (Dirs, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Dirs{}, err
	}
	return Dirs{
		Config: filepath.Join(base("XDG_CONFIG_HOME", home, ".config"), app),
		State:  filepath.Join(base("XDG_STATE_HOME", home, ".local", "state"), app),
		Data:   filepath.Join(base("XDG_DATA_HOME", home, ".local", "share"), app),
	}, nil
}

func base(env, home string, fallback ...string) string {
	if v := os.Getenv(env); filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(append([]string{home}, fallback...)...)
}
