// Package navigator is the CodeNavigator port: definitions, usages and
// symbols for code in a worktree, and whether that is still being indexed.
package navigator

import (
	"context"
	"errors"
)

// ErrNoServer is returned when no Language Server is available for a
// language.
var ErrNoServer = errors.New("no language server")

// Workspace is a directory of code in one language, served by one Language
// Server.
type Workspace struct {
	Root     string // absolute path
	Language string // e.g. "go"
}

// Position is a place in a file: a 1-based line and a 0-based column in
// UTF-16 code units, as the browser and LSP count them.
type Position struct {
	Line, Col int
}

// Location is a range on one line of a file, with Path relative to the
// Workspace root. Locations outside the root keep an absolute Path.
type Location struct {
	Path           string
	Line           int
	Col, EndCol    int
	OutsideProject bool // e.g. in the standard library or module cache
}

// Symbol is a named declaration found by a workspace symbol search.
type Symbol struct {
	Name      string
	Kind      string // e.g. "function", "struct"
	Container string // e.g. the package or type it is declared in
	Location
}

// Phase is where a Workspace's Language Server is in its life.
type Phase string

const (
	Stopped  Phase = "stopped"  // not running; it starts when needed
	Starting Phase = "starting" // launching and initialising
	Indexing Phase = "indexing" // running, but not yet able to answer
	Ready    Phase = "ready"
	Failed   Phase = "failed" // could not start; see Err
)

// ServerState is a Workspace's Language Server's Phase.
type ServerState struct {
	Phase  Phase
	Server string // the server's executable name, e.g. "gopls"
	Err    string // why it Failed
}

// Busy reports whether the server is on its way to Ready.
func (s ServerState) Busy() bool { return s.Phase == Starting || s.Phase == Indexing }

// Navigator answers code navigation queries.
type Navigator interface {
	Definition(ctx context.Context, ws Workspace, path string, pos Position) ([]Location, error)
	References(ctx context.Context, ws Workspace, path string, pos Position) ([]Location, error)
	Symbols(ctx context.Context, ws Workspace, query string) ([]Symbol, error)
	// Available reports whether a Language Server exists for language.
	Available(language string) bool
	// Warm starts the Workspace's server in the background so it has
	// indexed before the first query; ErrNoServer when there is none.
	Warm(ctx context.Context, ws Workspace) error
	// State reports the Workspace's server's Phase.
	State(ws Workspace) ServerState
	// Reset stops the Workspace's server, e.g. after its files changed on
	// disk; the next query starts a fresh one.
	Reset(ws Workspace)
}
