package release_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/release"
)

func TestNewVersion(t *testing.T) {
	for _, tc := range []struct {
		current, latest, want string
	}{
		{"v0.2.0", "v0.3.0", "v0.3.0"},
		{"v0.2.0", "v0.2.0", ""},
		{"v0.3.0", "v0.2.0", ""},
		{"v0.2.0", "v0.10.0", "v0.10.0"},
		{"v1.2.0-rc.1", "v1.2.0", "v1.2.0"},
		{"v0.2.0", "v0.3.0-rc.1", ""},
		{"v0.2.0", "", ""},
		{"(devel)", "v0.3.0", ""},
		{"v0.2.1-0.20261112093000-7d7b0b3abcde", "v0.3.0", ""},
		{"v0.2.0+dirty", "v0.3.0", ""},
	} {
		c := &release.Checker{Source: &release.Fake{Version: tc.latest}, Current: tc.current}
		c.Check(context.Background())
		if got := c.NewVersion(); got != tc.want {
			t.Errorf("current %q, latest %q: NewVersion() = %q, want %q", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestCheckKeepsLastVersionOnError(t *testing.T) {
	src := &release.Fake{Version: "v0.3.0"}
	c := &release.Checker{Source: src, Current: "v0.2.0"}
	c.Check(context.Background())
	src.Err = errors.New("offline")
	c.Check(context.Background())
	if got := c.NewVersion(); got != "v0.3.0" {
		t.Errorf("NewVersion() = %q, want v0.3.0", got)
	}
}

func TestNilCheckerHasNoNewVersion(t *testing.T) {
	var c *release.Checker
	if got := c.NewVersion(); got != "" {
		t.Errorf("NewVersion() = %q, want empty", got)
	}
}

func TestIsTagged(t *testing.T) {
	for v, want := range map[string]bool{
		"v0.1.0":                               true,
		"v1.2.0-rc.1":                          true,
		"v0.1.1-0.20261112093000-7d7b0b3abcde": false,
		"v0.0.0-20261112093000-7d7b0b3abcde":   false,
		"v0.1.0+dirty":                         false,
		"(devel)":                              false,
		"":                                     false,
	} {
		if got := release.IsTagged(v); got != want {
			t.Errorf("IsTagged(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestDevBuildsNeverCheck(t *testing.T) {
	for _, current := range []string{"(devel)", "", "v0.2.1-0.20261112093000-7d7b0b3abcde", "v0.2.0+dirty"} {
		src := &release.Fake{Version: "v0.3.0"}
		c := &release.Checker{Source: src, Current: current}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c.Run(ctx, time.Hour)
		if src.Calls != 0 {
			t.Errorf("current %q: Run asked the Source %d times, want 0", current, src.Calls)
		}
	}
}
