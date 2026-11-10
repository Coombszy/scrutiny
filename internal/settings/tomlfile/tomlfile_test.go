package tomlfile_test

import (
	"os"
	"path/filepath"
	"reflect"
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
	if !reflect.DeepEqual(got, settings.Defaults()) {
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
	if got.Port != 7878 || got.Theme != "gruvbox-dark" || got.Diff.CollapseThreshold != 1000 || got.Diff.ExpandLines != 64 {
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
	if !reflect.DeepEqual(got, want) {
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

func TestKeysMergeOverDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "[keys]\nnext_comment = \"n\"\n")

	got, err := tomlfile.New(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Keys["next_comment"] != "n" || got.Keys["symbol_search"] != "ctrl+k" || got.Keys["prev_comment"] != "[ c" || got.Keys["help"] != "?" {
		t.Errorf("Keys = %v", got.Keys)
	}
}

func TestInvalidKeysAreRejected(t *testing.T) {
	for _, bad := range []string{"[keys]\nfly = \"f\"\n", "[keys]\nrefresh = \"ctrl+\"\n", "[keys]\nrefresh = \"hyper+r\"\n", "[keys]\nrefresh = \"\"\n"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, bad)
		if _, err := tomlfile.New(path).Load(); err == nil {
			t.Errorf("Load(%q) succeeded, want an error", bad)
		}
	}
}

func TestConflictingKeysAreRejected(t *testing.T) {
	for _, bad := range []string{
		"[keys]\nrefresh = \"s\"\n",      // same as toggle_view_mode
		"[keys]\nrefresh = \"s x\"\n",    // "s" would always fire first
		"[keys]\nnext_comment = \"]\"\n", // prefix of nothing, but "[ c" is fine; "]" vs "] c"? next is rebound, so no clash
	} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, bad)
		_, err := tomlfile.New(path).Load()
		if bad == "[keys]\nnext_comment = \"]\"\n" {
			if err != nil {
				t.Errorf("Load(%q) = %v, want ok", bad, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("Load(%q) succeeded, want a conflict error", bad)
		}
	}
}

func TestExpandLinesMustBePositive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, "[diff]\nexpand_lines = 0\n")
	if _, err := tomlfile.New(path).Load(); err == nil {
		t.Error("expand_lines = 0 was accepted")
	}
}
