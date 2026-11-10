package live

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

// Source is what the Poller polls: the review service and code host health.
type Source interface {
	Poll(ctx context.Context, ref domain.PRRef) (review.Update, error)
	Status() codehost.Status
}

// backoff stretches the interval while the rate limit is low.
const backoff = 4

// Poller polls watched PRs every Interval and publishes what changed.
type Poller struct {
	Source   Source
	Hub      *Hub
	Interval time.Duration
	Now      func() time.Time

	last map[domain.PRRef]time.Time
}

// Run polls until ctx is done, checking for due PRs every second so that a
// newly connected page is served promptly.
func (p *Poller) Run(ctx context.Context) {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p.PollDue(ctx)
		}
	}
}

// PollDue polls every watched PR not polled within the interval.
func (p *Poller) PollDue(ctx context.Context) {
	if p.last == nil {
		p.last = map[domain.PRRef]time.Time{}
	}
	interval := p.Interval
	if p.Source.Status().RateLimit.Low() {
		interval *= backoff
	}
	now := p.Now()
	watched := p.Hub.Watched()
	for ref, last := range p.last {
		if now.Sub(last) > interval && !slices.Contains(watched, ref) {
			delete(p.last, ref) // forget PRs nobody is looking at any more
		}
	}
	for _, ref := range watched {
		if last, ok := p.last[ref]; ok && now.Sub(last) < interval {
			continue
		}
		p.last[ref] = now
		u, err := p.Source.Poll(ctx, ref)
		if err != nil {
			slog.Warn("poll failed", "pr", ref, "err", err)
			continue
		}
		if !u.Empty() {
			p.Hub.Publish(ref, u)
		}
	}
}
