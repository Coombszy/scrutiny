package release

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

// Checker looks for a New Version: a release newer than Current.
type Checker struct {
	Source  Source
	Current string // the running build's version

	mu     sync.Mutex
	latest string
}

// Enabled reports whether Current is a release build worth checking for: dev
// builds ("(devel)", pseudo-versions, dirty "+" builds) never look.
func (c *Checker) Enabled() bool {
	return c != nil && IsTagged(c.Current) && semver.IsValid(c.Current)
}

// Run checks now and then every interval until ctx is done; it returns at
// once for a dev build, which never asks the Source.
func (c *Checker) Run(ctx context.Context, interval time.Duration) {
	if !c.Enabled() {
		return
	}
	c.Check(ctx)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			c.Check(ctx)
		}
	}
}

// Check asks the Source for the latest release, keeping the last one found
// if it cannot say.
func (c *Checker) Check(ctx context.Context) {
	v, err := c.Source.Latest(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Debug("release check failed", "err", err)
		}
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latest = v
}

// NewVersion is the latest release if it is newer than Current, otherwise
// "". Dev builds have no New Version, and prereleases are never offered.
func (c *Checker) NewVersion() string {
	if !c.Enabled() {
		return ""
	}
	c.mu.Lock()
	latest := c.latest
	c.mu.Unlock()
	if !semver.IsValid(latest) || semver.Prerelease(latest) != "" || semver.Build(latest) != "" {
		return ""
	}
	if semver.Compare(latest, c.Current) <= 0 {
		return ""
	}
	return latest
}
