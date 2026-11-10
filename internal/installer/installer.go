// Package installer is the ServerInstaller port: listing, installing and
// removing Language Servers, and detecting the runtimes they need.
package installer

import (
	"context"
	"errors"
)

var (
	// ErrMissingRuntime is returned when installing a package whose runtime
	// (e.g. node or go) is not on $PATH. Scrutiny never installs runtimes.
	ErrMissingRuntime = errors.New("missing runtime")
	// ErrUnsupported is returned when installing a package whose source or
	// platform this installer does not handle.
	ErrUnsupported = errors.New("unsupported package")
	// ErrNotFound is returned for a package not in the registry.
	ErrNotFound = errors.New("package not found")
)

// Package is a Language Server listed in the registry.
type Package struct {
	Name        string
	Description string
	Homepage    string
	Languages   []string // as the registry names them, e.g. "Go", "TypeScript"
	Version     string
	Source      string   // "npm", "golang", "github", or another the registry uses
	Requires    []string // runtimes the install needs on $PATH, e.g. "node", "npm"
	// Unsupported says why the package cannot be installed here; empty when
	// it can.
	Unsupported string
}

// Installed is a Language Server package installed on this machine.
type Installed struct {
	Name      string
	Version   string
	Languages []string
	Bin       map[string]string // executable name -> absolute path
}

// Installer manages installed Language Servers.
type Installer interface {
	// Packages lists the registry's Language Servers, refreshing the
	// registry when refresh is set or it is not cached yet.
	Packages(ctx context.Context, refresh bool) ([]Package, error)
	Installed() ([]Installed, error)
	// Install installs the named package's current version, replacing any
	// installed version.
	Install(ctx context.Context, name string) error
	Remove(name string) error
	// Missing returns the runtimes p needs that are not on $PATH.
	Missing(p Package) []string
}
