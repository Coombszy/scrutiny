package metrics

import (
	"context"
	"sync"
	"time"
)

// Memory is an in-memory Store for tests.
type Memory struct {
	mu     sync.Mutex
	counts map[key]int
}

var _ Store = (*Memory)(nil)

func NewMemory() *Memory { return &Memory{counts: map[key]int{}} }

func (m *Memory) AddCounts(_ context.Context, bs []Bucket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range bs {
		m.counts[key{b.Name, b.Minute.Unix()}] += b.Count
	}
	return nil
}

func (m *Memory) CountsSince(_ context.Context, since time.Time) ([]Bucket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Bucket
	for k, n := range m.counts {
		if k.minute >= since.Unix() {
			out = append(out, Bucket{Name: k.name, Minute: time.Unix(k.minute, 0).UTC(), Count: n})
		}
	}
	return out, nil
}

func (m *Memory) PruneCounts(_ context.Context, before time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k := range m.counts {
		if k.minute < before.Unix() {
			delete(m.counts, k)
		}
	}
	return nil
}
