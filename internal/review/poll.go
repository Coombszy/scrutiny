package review

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

// Update is what changed on a PR since it was last read.
type Update struct {
	// Head is the new head commit, set only when the head moved.
	Head domain.SHA
	// Threads are already-known Review Threads whose content changed;
	// NewThreads counts threads that appeared, which need a refresh to place.
	Threads    []domain.ReviewThread
	NewThreads int

	Conversation        []domain.Comment
	Reviews             []domain.SubmittedReview // with Conversation, when either changed
	ConversationChanged bool
	Pending             *domain.PendingReview
	PendingComments     []PendingComment // set with PendingChanged
	PendingChanged      bool
	Checks              *domain.Checks // set when the Checks changed

	Offline        bool
	OfflineChanged bool

	// Languages is set when a Language Server for the PR changes state.
	Languages []LanguageStatus
}

// Empty reports whether nothing changed.
func (u Update) Empty() bool {
	return u.Head == "" && len(u.Threads) == 0 && u.NewThreads == 0 && !u.ConversationChanged &&
		!u.PendingChanged && u.Checks == nil && !u.OfflineChanged && u.Languages == nil
}

// readFresh reads the review state and Checks, falling back to the cached
// snapshot (marked Offline) when the code host is unreachable.
func (s *Service) readFresh(ctx context.Context, ref domain.PRRef, head, mergeBase domain.SHA) (snapshot, error) {
	snap, err := s.readState(ctx, ref, head, mergeBase)
	if errors.Is(err, codehost.ErrOffline) {
		cached, ok, cerr := s.cachedSnapshot(ctx, ref)
		if cerr != nil || !ok {
			return snapshot{}, err
		}
		cached.Offline = true
		return cached, s.saveSnapshot(ctx, ref, cached)
	}
	if err != nil {
		return snapshot{}, err
	}
	checks, err := s.CodeHost.Checks(ctx, ref)
	if err != nil && !errors.Is(err, codehost.ErrOffline) {
		return snapshot{}, err
	}
	if err == nil {
		snap.Checks = checks
	}
	return snap, s.saveSnapshot(ctx, ref, snap)
}

// Poll re-reads the PR and reports what changed since the last read.
func (s *Service) Poll(ctx context.Context, ref domain.PRRef) (Update, error) {
	prev, err := s.snapshot(ctx, ref)
	if err != nil {
		return Update{}, err
	}
	pr, err := s.CodeHost.PullRequest(ctx, ref)
	if err != nil {
		return Update{}, err
	}
	head, mergeBase := prev.Head, prev.MergeBase
	var u Update
	if pr.HeadSHA != prev.Head && s.CodeHost.Status().Online {
		if mergeBase, err = s.mergeBase(ctx, pr); err != nil {
			return Update{}, err
		}
		head, u.Head = pr.HeadSHA, pr.HeadSHA
	}
	next, err := s.readFresh(ctx, ref, head, mergeBase)
	if err != nil {
		return Update{}, err
	}
	u.Offline, u.OfflineChanged = next.Offline, next.Offline != prev.Offline
	if next.Offline {
		u.Head = ""
		return u, nil
	}
	diffThreads(&u, prev.State.Threads, next.State.Threads)
	if !sameJSON(prev.State.Conversation, next.State.Conversation) || !sameJSON(prev.State.Reviews, next.State.Reviews) {
		u.Conversation, u.Reviews, u.ConversationChanged = next.State.Conversation, next.State.Reviews, true
	}
	if pending := pendingComments(next.State); !sameJSON(prev.State.Pending, next.State.Pending) ||
		!sameJSON(pendingComments(prev.State), pending) {
		u.Pending, u.PendingComments, u.PendingChanged = next.State.Pending, pending, true
	}
	if !sameJSON(prev.Checks, next.Checks) {
		u.Checks = &next.Checks
	}
	return u, nil
}

func diffThreads(u *Update, prev, next []domain.ReviewThread) {
	known := make(map[string]domain.ReviewThread, len(prev))
	for _, t := range prev {
		known[t.ID] = t
	}
	for _, t := range next {
		old, ok := known[t.ID]
		switch {
		case !ok:
			u.NewThreads++
		case !sameJSON(old, t):
			u.Threads = append(u.Threads, t)
		}
	}
	slices.SortFunc(u.Threads, func(a, b domain.ReviewThread) int { return cmp.Compare(a.ID, b.ID) })
}

func sameJSON(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// Status reports the code host's reachability and rate limit.
func (s *Service) Status() codehost.Status { return s.CodeHost.Status() }
