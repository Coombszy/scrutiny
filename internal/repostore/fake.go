package repostore

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// Pair is an ordered pair of commits, keying Fake's per-comparison data.
type Pair struct{ From, To domain.SHA }

// FakeRemote is what FetchPR makes available for one PR.
type FakeRemote struct {
	BaseTip domain.SHA
	Commits []domain.SHA
}

// Fake is an in-memory Store for tests, driven entirely by preset data.
type Fake struct {
	mu         sync.Mutex
	Present    map[domain.SHA]bool
	Remotes    map[domain.PRRef]FakeRemote
	MergeBases map[Pair]domain.SHA
	ChangeSets map[Pair][]domain.FileChange
	Diffs      map[Pair]map[string]domain.FileDiff // keyed by path
	Blobs      map[domain.SHA]map[string]string
	Attrs      map[string]map[string]string // attribute -> path -> value, at any SHA
	Fetches    int
	// Worktrees maps each PR with a worktree to the commit checked out.
	Worktrees map[domain.PRRef]domain.SHA
}

var _ Store = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{
		Present:    map[domain.SHA]bool{},
		Remotes:    map[domain.PRRef]FakeRemote{},
		MergeBases: map[Pair]domain.SHA{},
		ChangeSets: map[Pair][]domain.FileChange{},
		Diffs:      map[Pair]map[string]domain.FileDiff{},
		Blobs:      map[domain.SHA]map[string]string{},
		Attrs:      map[string]map[string]string{},
		Worktrees:  map[domain.PRRef]domain.SHA{},
	}
}

func (f *Fake) Worktree(_ context.Context, pr domain.PRRef, sha domain.SHA) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Present[sha] {
		return "", fmt.Errorf("worktree at %s: %w", sha.Short(), ErrNotFound)
	}
	f.Worktrees[pr] = sha
	return "/worktrees/" + pr.String(), nil
}

func (f *Fake) RemoveWorktree(_ context.Context, pr domain.PRRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Worktrees, pr)
	return nil
}

func (f *Fake) FetchPR(_ context.Context, pr domain.PRRef, _ string, _ domain.SHA) (domain.SHA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Fetches++
	r, ok := f.Remotes[pr]
	if !ok {
		return "", fmt.Errorf("fetch %s: %w", pr, ErrNotFound)
	}
	f.Present[r.BaseTip] = true
	for _, c := range r.Commits {
		f.Present[c] = true
	}
	return r.BaseTip, nil
}

func (f *Fake) HasCommit(_ context.Context, _ domain.Repo, sha domain.SHA) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Present[sha], nil
}

func (f *Fake) MergeBase(_ context.Context, _ domain.Repo, a, b domain.SHA) (domain.SHA, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if mb, ok := f.MergeBases[Pair{a, b}]; ok {
		return mb, nil
	}
	return "", fmt.Errorf("merge-base %s %s: %w", a.Short(), b.Short(), ErrNotFound)
}

func (f *Fake) Changes(_ context.Context, _ domain.Repo, from, to domain.SHA) ([]domain.FileChange, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cs, ok := f.ChangeSets[Pair{from, to}]
	if !ok {
		return nil, fmt.Errorf("changes %s..%s: %w", from.Short(), to.Short(), ErrNotFound)
	}
	return cs, nil
}

func (f *Fake) FileDiff(_ context.Context, _ domain.Repo, from, to domain.SHA, change domain.FileChange) (domain.FileDiff, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.Diffs[Pair{from, to}][change.Path]
	if !ok {
		return domain.FileDiff{}, fmt.Errorf("diff %s: %w", change.Path, ErrNotFound)
	}
	return d, nil
}

func (f *Fake) Blob(_ context.Context, _ domain.Repo, sha domain.SHA, path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.Blobs[sha][path]
	if !ok {
		return nil, fmt.Errorf("%s:%s: %w", sha.Short(), path, ErrNotFound)
	}
	return []byte(b), nil
}

func (f *Fake) ListFiles(_ context.Context, _ domain.Repo, sha domain.SHA) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var paths []string
	for p := range f.Blobs[sha] {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, nil
}

func (f *Fake) Attribute(_ context.Context, _ domain.Repo, _ domain.SHA, attr string, paths []string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	values := make(map[string]string, len(paths))
	for _, p := range paths {
		v, ok := f.Attrs[attr][p]
		if !ok {
			v = "unspecified"
		}
		values[p] = v
	}
	return values, nil
}
