package installer

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
)

// Fake is an in-memory Installer for tests.
type Fake struct {
	mu       sync.Mutex
	Registry []Package
	Have     map[string]Installed
	// Absent lists runtimes that are not on $PATH.
	Absent map[string]bool
	// Block, when set, makes Install wait until it is closed.
	Block chan struct{}
	Fail  error
}

var _ Installer = (*Fake)(nil)

func NewFake(registry ...Package) *Fake {
	return &Fake{Registry: registry, Have: map[string]Installed{}, Absent: map[string]bool{}}
}

func (f *Fake) Packages(context.Context, bool) ([]Package, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.Registry), nil
}

func (f *Fake) Installed() ([]Installed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Installed
	for _, in := range f.Have {
		out = append(out, in)
	}
	slices.SortFunc(out, func(a, b Installed) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

func (f *Fake) Install(ctx context.Context, name string) error {
	if f.Block != nil {
		select {
		case <-f.Block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return f.Fail
	}
	i := slices.IndexFunc(f.Registry, func(p Package) bool { return p.Name == name })
	if i < 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	p := f.Registry[i]
	bins := map[string]string{}
	bins[p.Name] = "/servers/bin/" + p.Name
	f.Have[name] = Installed{Name: name, Version: p.Version, Languages: p.Languages, Bin: bins}
	return nil
}

func (f *Fake) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Have, name)
	return nil
}

func (f *Fake) Missing(p Package) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var missing []string
	for _, r := range p.Requires {
		if f.Absent[r] {
			missing = append(missing, r)
		}
	}
	return missing
}
