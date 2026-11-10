// Package live pushes PR changes to connected browsers. A browser holds a
// connection only while its page is visible, so a PR with subscribers is a
// PR someone is looking at, and only those are polled.
package live

import (
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

// Hub tracks connections per PR and fans Updates out to them.
type Hub struct {
	mu   sync.Mutex
	subs map[domain.PRRef]map[chan review.Update]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: map[domain.PRRef]map[chan review.Update]struct{}{}}
}

// Subscribe registers a connection to ref. The returned func unsubscribes.
func (h *Hub) Subscribe(ref domain.PRRef) (<-chan review.Update, func()) {
	ch := make(chan review.Update, 8)
	h.mu.Lock()
	if h.subs[ref] == nil {
		h.subs[ref] = map[chan review.Update]struct{}{}
	}
	h.subs[ref][ch] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.subs[ref], ch)
			if len(h.subs[ref]) == 0 {
				delete(h.subs, ref)
			}
		})
	}
}

// Watched returns the PRs with at least one connection.
func (h *Hub) Watched() []domain.PRRef {
	h.mu.Lock()
	defer h.mu.Unlock()
	refs := make([]domain.PRRef, 0, len(h.subs))
	for ref := range h.subs {
		refs = append(refs, ref)
	}
	return refs
}

// Publish sends u to every connection to ref, dropping it for any that is
// too far behind to keep up.
func (h *Hub) Publish(ref domain.PRRef, u review.Update) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[ref] {
		select {
		case ch <- u:
		default:
		}
	}
}
