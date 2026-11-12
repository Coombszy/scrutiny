package release

import (
	"context"
	"sync"
)

// Fake is an in-memory Source for tests.
type Fake struct {
	mu      sync.Mutex
	Version string
	Err     error
	Calls   int
}

func (f *Fake) Latest(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls++
	return f.Version, f.Err
}

// Set changes the latest version.
func (f *Fake) Set(v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Version = v
}
