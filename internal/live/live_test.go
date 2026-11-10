package live_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/live"
	"github.com/Coombszy/scrutiny/internal/review"
)

var (
	pr1 = domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: "o", Name: "r"}, Number: 1}
	pr2 = domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: "o", Name: "r"}, Number: 2}
)

// source counts polls per PR and returns the queued Update for each.
type source struct {
	mu      sync.Mutex
	polls   map[domain.PRRef]int
	updates map[domain.PRRef]review.Update
	status  codehost.Status
}

func newSource() *source {
	return &source{polls: map[domain.PRRef]int{}, updates: map[domain.PRRef]review.Update{}, status: codehost.Status{Online: true}}
}

func (s *source) Poll(_ context.Context, ref domain.PRRef) (review.Update, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polls[ref]++
	u := s.updates[ref]
	delete(s.updates, ref)
	return u, nil
}

func (s *source) Status() codehost.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *source) count(ref domain.PRRef) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.polls[ref]
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time          { return c.now }
func (c *clock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newPoller(src *source, hub *live.Hub, c *clock) *live.Poller {
	return &live.Poller{Source: src, Hub: hub, Interval: 30 * time.Second, Now: c.Now}
}

func TestOnlyWatchedPRsArePolled(t *testing.T) {
	src, hub, c := newSource(), live.NewHub(), &clock{now: time.Unix(1000, 0)}
	p := newPoller(src, hub, c)
	ctx := context.Background()

	p.PollDue(ctx)
	if src.count(pr1) != 0 {
		t.Fatal("polled with no browser connected")
	}

	_, leave := hub.Subscribe(pr1)
	p.PollDue(ctx)
	if src.count(pr1) != 1 || src.count(pr2) != 0 {
		t.Fatalf("polls = %v, want pr1 polled once on connect", src.polls)
	}

	c.advance(10 * time.Second)
	p.PollDue(ctx)
	if src.count(pr1) != 1 {
		t.Error("polled again before the interval")
	}
	c.advance(25 * time.Second)
	p.PollDue(ctx)
	if src.count(pr1) != 2 {
		t.Error("not polled after the interval")
	}

	leave()
	c.advance(time.Minute)
	p.PollDue(ctx)
	if src.count(pr1) != 2 {
		t.Error("polled after the browser disconnected")
	}
}

func TestReconnectingSoonAfterAPollDoesNotPollAgain(t *testing.T) {
	src, hub, c := newSource(), live.NewHub(), &clock{now: time.Unix(1000, 0)}
	p := newPoller(src, hub, c)
	ctx := context.Background()
	_, leave := hub.Subscribe(pr1)
	p.PollDue(ctx)
	leave()

	// The page is hidden and shown again a few seconds later.
	c.advance(5 * time.Second)
	_, leave = hub.Subscribe(pr1)
	defer leave()
	p.PollDue(ctx)
	if src.count(pr1) != 1 {
		t.Errorf("polls = %d, want 1", src.count(pr1))
	}
}

func TestLowRateLimitBacksOff(t *testing.T) {
	src, hub, c := newSource(), live.NewHub(), &clock{now: time.Unix(1000, 0)}
	p := newPoller(src, hub, c)
	ctx := context.Background()
	_, leave := hub.Subscribe(pr1)
	defer leave()
	src.status.RateLimit = codehost.RateLimit{Remaining: 400, Limit: 5000}

	p.PollDue(ctx)
	c.advance(31 * time.Second)
	p.PollDue(ctx)
	if src.count(pr1) != 1 {
		t.Errorf("polls = %d, want the interval stretched while the rate limit is low", src.count(pr1))
	}
	c.advance(2 * time.Minute)
	p.PollDue(ctx)
	if src.count(pr1) != 2 {
		t.Errorf("polls = %d, want 2 after the backed-off interval", src.count(pr1))
	}
}

func TestUpdatesAreSentToEverySubscriberOfThatPR(t *testing.T) {
	src, hub, c := newSource(), live.NewHub(), &clock{now: time.Unix(1000, 0)}
	p := newPoller(src, hub, c)
	a, leaveA := hub.Subscribe(pr1)
	defer leaveA()
	b, leaveB := hub.Subscribe(pr1)
	defer leaveB()
	other, leaveOther := hub.Subscribe(pr2)
	defer leaveOther()
	src.updates[pr1] = review.Update{Head: "abc"}

	p.PollDue(context.Background())

	for name, ch := range map[string]<-chan review.Update{"a": a, "b": b} {
		select {
		case u := <-ch:
			if u.Head != "abc" {
				t.Errorf("%s got %+v", name, u)
			}
		default:
			t.Errorf("%s got nothing", name)
		}
	}
	select {
	case u := <-other:
		t.Errorf("pr2 subscriber got %+v (empty updates must not be sent)", u)
	default:
	}
}
