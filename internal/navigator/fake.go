package navigator

import (
	"context"
	"fmt"
	"sync"
)

// Query identifies a navigation request in the Fake.
type Query struct {
	Path string
	Pos  Position
}

// Fake is an in-memory Navigator for tests, answering from preset results
// for any Workspace whose Language is in Languages.
type Fake struct {
	mu          sync.Mutex
	Languages   map[string]bool
	Definitions map[Query][]Location
	Refs        map[Query][]Location
	AllSymbols  []Symbol
	States      map[Workspace]ServerState
	Logs        map[string][]LogEntry // by executable path
	// Calls records the Workspace of every query; Resets every Reset.
	Calls  []Workspace
	Resets []Workspace
	Warmed []Workspace
}

var _ Navigator = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{
		Languages:   map[string]bool{"go": true},
		Definitions: map[Query][]Location{},
		Refs:        map[Query][]Location{},
		States:      map[Workspace]ServerState{},
		Logs:        map[string][]LogEntry{},
	}
}

// Log returns the preset log for executable.
func (f *Fake) Log(executable string) []LogEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Logs[executable]
}

func (f *Fake) check(ws Workspace) error {
	f.Calls = append(f.Calls, ws)
	if !f.Languages[ws.Language] {
		return fmt.Errorf("%w for %s", ErrNoServer, ws.Language)
	}
	return nil
}

func (f *Fake) Definition(_ context.Context, ws Workspace, path string, pos Position) ([]Location, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.check(ws); err != nil {
		return nil, err
	}
	return f.Definitions[Query{path, pos}], nil
}

func (f *Fake) References(_ context.Context, ws Workspace, path string, pos Position) ([]Location, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.check(ws); err != nil {
		return nil, err
	}
	return f.Refs[Query{path, pos}], nil
}

func (f *Fake) Symbols(_ context.Context, ws Workspace, _ string) ([]Symbol, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.check(ws); err != nil {
		return nil, err
	}
	return f.AllSymbols, nil
}

func (f *Fake) Available(language string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Languages[language]
}

// Warm records the Workspace and reports it Starting, as a real server would.
func (f *Fake) Warm(_ context.Context, ws Workspace) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Languages[ws.Language] {
		return fmt.Errorf("%w for %s", ErrNoServer, ws.Language)
	}
	f.Warmed = append(f.Warmed, ws)
	if _, ok := f.States[ws]; !ok {
		f.States[ws] = ServerState{Phase: Starting, Server: ws.Language + "-server"}
	}
	return nil
}

func (f *Fake) State(ws Workspace) ServerState {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.States[ws]; ok {
		return st
	}
	return ServerState{Phase: Stopped}
}

func (f *Fake) Reset(ws Workspace) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Resets = append(f.Resets, ws)
}
