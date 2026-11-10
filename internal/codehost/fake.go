package codehost

import (
	"context"
	"fmt"
	"html"
	"slices"
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// Submission records a review submitted to the Fake.
type Submission struct {
	PR     domain.PRRef
	Commit domain.SHA
	Event  domain.ReviewEvent
	Body   string
}

// Fake is an in-memory CodeHost for tests. Its review writes update States
// the way GitHub would, so tests can read the effect back.
type Fake struct {
	mu          sync.Mutex
	PRs         map[domain.PRRef]domain.PullRequest
	CommitsOf   map[domain.PRRef][]domain.Commit
	Dash        Dashboard
	States      map[domain.PRRef]*domain.ReviewState
	Submissions []Submission
	ChecksOf    map[domain.PRRef]domain.Checks
	Health      Status
	// Forgotten lists the PRs Forget was called for.
	Forgotten []domain.PRRef
	// FailAddThread, when set, makes AddThread fail with it.
	FailAddThread error
	nextID        int
}

var _ CodeHost = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{
		PRs:       map[domain.PRRef]domain.PullRequest{},
		CommitsOf: map[domain.PRRef][]domain.Commit{},
		States:    map[domain.PRRef]*domain.ReviewState{},
		ChecksOf:  map[domain.PRRef]domain.Checks{},
		Health:    Status{Online: true},
	}
}

// SetOnline sets whether the Fake behaves as reachable. Unreachable, it acts
// like the GitHub adapter: REST reads (PRs, commits, Dashboard) still succeed
// as if served from cache, while GraphQL reads and all writes fail with
// ErrOffline.
func (f *Fake) SetOnline(online bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Health.Online = online
}

func (f *Fake) Status() Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Health
}

func (f *Fake) Checks(_ context.Context, ref domain.PRRef) (domain.Checks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return domain.Checks{}, ErrOffline
	}
	pr, ok := f.PRs[ref]
	if !ok {
		return domain.Checks{}, ErrNotFound
	}
	c := f.ChecksOf[ref]
	c.PRState = pr.State
	return c, nil
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

// State returns the review state for ref, creating an empty one.
func (f *Fake) State(ref domain.PRRef) *domain.ReviewState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state(ref)
}

func (f *Fake) state(ref domain.PRRef) *domain.ReviewState {
	s, ok := f.States[ref]
	if !ok {
		s = &domain.ReviewState{PRID: "PR_" + ref.String(), Viewer: "me", Viewed: map[string]domain.ViewedState{}}
		f.States[ref] = s
	}
	return s
}

func (f *Fake) ReviewState(_ context.Context, ref domain.PRRef) (domain.ReviewState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return domain.ReviewState{}, ErrOffline
	}
	if _, ok := f.PRs[ref]; !ok {
		return domain.ReviewState{}, ErrNotFound
	}
	return clone(*f.state(ref)), nil
}

// clone deep-copies s so callers cannot alias the Fake's state.
func clone(s domain.ReviewState) domain.ReviewState {
	out := s
	out.Threads = make([]domain.ReviewThread, len(s.Threads))
	for i, t := range s.Threads {
		t.Comments = cloneComments(t.Comments)
		out.Threads[i] = t
	}
	out.Conversation = cloneComments(s.Conversation)
	out.Reactions = append([]domain.ReactionGroup(nil), s.Reactions...)
	out.Viewed = make(map[string]domain.ViewedState, len(s.Viewed))
	for k, v := range s.Viewed {
		out.Viewed[k] = v
	}
	if s.Pending != nil {
		p := *s.Pending
		out.Pending = &p
	}
	return out
}

func cloneComments(cs []domain.Comment) []domain.Comment {
	out := make([]domain.Comment, len(cs))
	for i, c := range cs {
		c.Reactions = append([]domain.ReactionGroup(nil), c.Reactions...)
		out[i] = c
	}
	return out
}

func (f *Fake) id(prefix string) string {
	f.nextID++
	return fmt.Sprintf("%s_%d", prefix, f.nextID)
}

func (f *Fake) byPRID(prID string) (domain.PRRef, *domain.ReviewState, error) {
	for ref, s := range f.States {
		if s.PRID == prID {
			return ref, s, nil
		}
	}
	return domain.PRRef{}, nil, fmt.Errorf("PR %s: %w", prID, ErrNotFound)
}

func (f *Fake) byReviewID(reviewID string) (*domain.ReviewState, error) {
	for _, s := range f.States {
		if s.Pending != nil && s.Pending.ID == reviewID {
			return s, nil
		}
	}
	return nil, fmt.Errorf("review %s: %w", reviewID, ErrNotFound)
}

func (f *Fake) StartReview(_ context.Context, prID string, commit domain.SHA) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return "", ErrOffline
	}
	_, s, err := f.byPRID(prID)
	if err != nil {
		return "", err
	}
	if s.Pending != nil {
		return "", fmt.Errorf("a pending review already exists")
	}
	s.Pending = &domain.PendingReview{ID: f.id("PRR"), Commit: commit}
	return s.Pending.ID, nil
}

func (f *Fake) AddThread(_ context.Context, reviewID string, t NewThread) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	if f.FailAddThread != nil {
		return f.FailAddThread
	}
	s, err := f.byReviewID(reviewID)
	if err != nil {
		return err
	}
	s.Threads = append(s.Threads, domain.ReviewThread{
		ID: f.id("PRRT"), Path: t.Path, Line: t.Line, StartLine: t.StartLine, Side: t.Side, StartSide: t.StartSide,
		ViewerCanResolve: true,
		Comments:         []domain.Comment{{ID: f.id("PRRC"), Author: s.Viewer, Body: t.Body, BodyHTML: renderBody(t.Body), Pending: true, ViewerDidAuthor: true, ViewerCanUpdate: true, ViewerCanDelete: true}},
	})
	s.Pending.Comments++
	return nil
}

func (f *Fake) Reply(_ context.Context, reviewID, threadID, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	s, err := f.byReviewID(reviewID)
	if err != nil {
		return err
	}
	for i := range s.Threads {
		if s.Threads[i].ID == threadID {
			s.Threads[i].Comments = append(s.Threads[i].Comments, domain.Comment{
				ID: f.id("PRRC"), Author: s.Viewer, Body: body, BodyHTML: renderBody(body), Pending: true, ViewerDidAuthor: true, ViewerCanUpdate: true, ViewerCanDelete: true,
			})
			s.Pending.Comments++
			return nil
		}
	}
	return fmt.Errorf("thread %s: %w", threadID, ErrNotFound)
}

func (f *Fake) SetThreadResolved(_ context.Context, threadID string, resolved bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	for _, s := range f.States {
		for i := range s.Threads {
			if s.Threads[i].ID == threadID {
				s.Threads[i].Resolved = resolved
				s.Threads[i].ViewerCanResolve, s.Threads[i].ViewerCanUnresolve = !resolved, resolved
				return nil
			}
		}
	}
	return fmt.Errorf("thread %s: %w", threadID, ErrNotFound)
}

func (f *Fake) AddConversationComment(_ context.Context, prID, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	_, s, err := f.byPRID(prID)
	if err != nil {
		return err
	}
	s.Conversation = append(s.Conversation, domain.Comment{ID: f.id("IC"), Author: s.Viewer, Body: body, BodyHTML: renderBody(body), ViewerDidAuthor: true, ViewerCanUpdate: true, ViewerCanDelete: true})
	return nil
}

func (f *Fake) SetReaction(_ context.Context, subjectID string, r domain.Reaction, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	for _, s := range f.States {
		if s.PRID == subjectID {
			s.Reactions = react(s.Reactions, r, on)
			return nil
		}
		for i := range s.Conversation {
			if s.Conversation[i].ID == subjectID {
				s.Conversation[i].Reactions = react(s.Conversation[i].Reactions, r, on)
				return nil
			}
		}
		for i := range s.Threads {
			for j := range s.Threads[i].Comments {
				if c := &s.Threads[i].Comments[j]; c.ID == subjectID {
					c.Reactions = react(c.Reactions, r, on)
					return nil
				}
			}
		}
	}
	return fmt.Errorf("subject %s: %w", subjectID, ErrNotFound)
}

func react(groups []domain.ReactionGroup, r domain.Reaction, on bool) []domain.ReactionGroup {
	for i, g := range groups {
		if g.Content == r {
			if g.ViewerHasReacted != on {
				groups[i].ViewerHasReacted = on
				if on {
					groups[i].Count++
				} else {
					groups[i].Count--
				}
			}
			return groups
		}
	}
	if on {
		groups = append(groups, domain.ReactionGroup{Content: r, Count: 1, ViewerHasReacted: true})
	}
	return groups
}

func (f *Fake) SubmitReview(_ context.Context, prID, reviewID string, commit domain.SHA, event domain.ReviewEvent, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	ref, s, err := f.byPRID(prID)
	if err != nil {
		return err
	}
	if reviewID != "" {
		if s.Pending == nil || s.Pending.ID != reviewID {
			return fmt.Errorf("review %s: %w", reviewID, ErrNotFound)
		}
		commit = s.Pending.Commit
	}
	s.Pending = nil
	for i := range s.Threads {
		for j := range s.Threads[i].Comments {
			s.Threads[i].Comments[j].Pending = false
		}
	}
	f.Submissions = append(f.Submissions, Submission{PR: ref, Commit: commit, Event: event, Body: body})
	return nil
}

func (f *Fake) SetViewed(_ context.Context, prID, path string, viewed bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	_, s, err := f.byPRID(prID)
	if err != nil {
		return err
	}
	if viewed {
		s.Viewed[path] = domain.Viewed
	} else {
		s.Viewed[path] = domain.Unviewed
	}
	return nil
}

// renderBody stands in for GitHub's Markdown rendering: escaped text in a
// paragraph.
func renderBody(body string) string { return "<p>" + html.EscapeString(body) + "</p>" }

func (f *Fake) Forget(_ context.Context, ref domain.PRRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Forgotten = append(f.Forgotten, ref)
	return nil
}

func (f *Fake) EditReviewComment(_ context.Context, commentID, body string) error {
	return f.editComment(commentID, func(c *domain.Comment) { c.Body, c.BodyHTML = body, renderBody(body) })
}

// DeleteReviewComment removes the comment, and its thread once empty, as
// GitHub does.
func (f *Fake) DeleteReviewComment(_ context.Context, commentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	for _, s := range f.States {
		for i := range s.Threads {
			t := &s.Threads[i]
			for j, c := range t.Comments {
				if c.ID != commentID {
					continue
				}
				t.Comments = slices.Delete(t.Comments, j, j+1)
				if c.Pending && s.Pending != nil {
					s.Pending.Comments--
				}
				if len(t.Comments) == 0 {
					s.Threads = slices.Delete(s.Threads, i, i+1)
				}
				return nil
			}
		}
	}
	return fmt.Errorf("comment %s: %w", commentID, ErrNotFound)
}

func (f *Fake) EditConversationComment(_ context.Context, commentID, body string) error {
	return f.editComment(commentID, func(c *domain.Comment) { c.Body, c.BodyHTML = body, renderBody(body) })
}

func (f *Fake) DeleteConversationComment(_ context.Context, commentID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	for _, s := range f.States {
		for i, c := range s.Conversation {
			if c.ID == commentID {
				s.Conversation = slices.Delete(s.Conversation, i, i+1)
				return nil
			}
		}
	}
	return fmt.Errorf("comment %s: %w", commentID, ErrNotFound)
}

// editComment applies change to the comment with id, wherever it is.
func (f *Fake) editComment(id string, change func(*domain.Comment)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.Health.Online {
		return ErrOffline
	}
	for _, s := range f.States {
		for i := range s.Conversation {
			if s.Conversation[i].ID == id {
				change(&s.Conversation[i])
				return nil
			}
		}
		for i := range s.Threads {
			for j := range s.Threads[i].Comments {
				if s.Threads[i].Comments[j].ID == id {
					change(&s.Threads[i].Comments[j])
					return nil
				}
			}
		}
	}
	return fmt.Errorf("comment %s: %w", id, ErrNotFound)
}
