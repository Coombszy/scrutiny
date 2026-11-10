package codehost

import (
	"context"
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// Fake is an in-memory CodeHost for tests.
type Fake struct {
	mu        sync.Mutex
	PRs       map[domain.PRRef]domain.PullRequest
	CommitsOf map[domain.PRRef][]domain.Commit
	Dash      Dashboard
}

var _ CodeHost = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{PRs: map[domain.PRRef]domain.PullRequest{}, CommitsOf: map[domain.PRRef][]domain.Commit{}}
}

func (f *Fake) PullRequest(_ context.Context, ref domain.PRRef) (domain.PullRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pr, ok := f.PRs[ref]
	if !ok {
		return domain.PullRequest{}, ErrNotFound
	}
	return pr, nil
}

func (f *Fake) Commits(_ context.Context, ref domain.PRRef) ([]domain.Commit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.PRs[ref]; !ok {
		return nil, ErrNotFound
	}
	return f.CommitsOf[ref], nil
}

func (f *Fake) Dashboard(context.Context) (Dashboard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Dash, nil
}
