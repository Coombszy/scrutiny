package xdg_test

import (
	"path/filepath"
	"testing"

	"github.com/Coombszy/scrutiny/internal/xdg"
)

func TestDirsRespectXDGEnv(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("XDG_DATA_HOME", "/data")

	d, err := xdg.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := d.Config, "/cfg/scrutiny"; got != want {
		t.Errorf("Config = %q, want %q", got, want)
	}
	if got, want := d.State, filepath.FromSlash("/home/u/.local/state/scrutiny"); got != want {
		t.Errorf("State = %q, want %q", got, want)
	}
	if got, want := d.Data, "/data/scrutiny"; got != want {
		t.Errorf("Data = %q, want %q", got, want)
	}
}

func TestRelativeXDGEnvIsIgnored(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("XDG_CONFIG_HOME", "relative/path")

	d, err := xdg.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := d.Config, "/home/u/.config/scrutiny"; got != want {
		t.Errorf("Config = %q, want %q", got, want)
	}
}
