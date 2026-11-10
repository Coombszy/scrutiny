package tomlfile_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/settings/tomlfile"
)

func TestMissingFileLoadsDefaults(t *testing.T) {
	store := tomlfile.New(filepath.Join(t.TempDir(), "config.toml"))

	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != settings.Defaults() {
		t.Errorf("Load() = %+v, want defaults %+v", got, settings.Defaults())
	}
}

func TestPartialFileKeepsDefaultsForUnsetFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "view_mode = \"unified\"\n")

	got, err := tomlfile.New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ViewMode != settings.Unified {
		t.Errorf("ViewMode = %q, want unified", got.ViewMode)
	}
	if got.Port != 7878 || got.Theme != "gruvbox-dark" || got.Diff.CollapseThreshold != 1000 {
		t.Errorf("unset fields lost their defaults: %+v", got)
	}
}

func TestSaveThenLoadRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	store := tomlfile.New(path)
	want := settings.Defaults()
	want.Port = 9000
	want.Diff.CollapseThreshold = 250

	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestInvalidViewModeIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "view_mode = \"sideways\"\n")

	if _, err := tomlfile.New(path).Load(); err == nil {
		t.Fatal("Load() succeeded, want an error for an unknown view_mode")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
