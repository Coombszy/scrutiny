package cache

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// Memory is an in-memory Cache for tests.
type Memory struct {
	mu      sync.Mutex
	entries map[string]Entry
	visits  map[domain.PRRef]Visit
}

var _ Cache = (*Memory)(nil)

func NewMemory() *Memory {
	return &Memory{entries: map[string]Entry{}, visits: map[domain.PRRef]Visit{}}
}

func (m *Memory) Get(_ context.Context, key string) (Entry, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[key]
	return e, ok, nil
}

func (m *Memory) Put(_ context.Context, key string, e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = e
	return nil
}

func (m *Memory) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, key)
	return nil
}

func (m *Memory) DeletePrefix(_ context.Context, prefix string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.entries {
		if strings.HasPrefix(key, prefix) {
			delete(m.entries, key)
		}
	}
	return nil
}

// Keys returns the keys of every entry, for tests.
func (m *Memory) Keys() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Sorted(maps.Keys(m.entries))
}

func (m *Memory) RecordVisit(_ context.Context, v Visit) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.visits[v.PR] = v
	return nil
}

func (m *Memory) RecentVisits(_ context.Context, limit int) ([]Visit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var vs []Visit
	for _, v := range m.visits {
		vs = append(vs, v)
	}
	slices.SortFunc(vs, func(a, b Visit) int { return b.VisitedAt.Compare(a.VisitedAt) })
	if len(vs) > limit {
		vs = vs[:limit]
	}
	return vs, nil
}

func (m *Memory) DeleteVisit(_ context.Context, pr domain.PRRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.visits, pr)
	return nil
}
