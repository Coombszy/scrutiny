// Package codehost is the CodeHost port: Pull Requests and review state on
// the code host, which is the source of truth for review state.
package codehost

import (
	"context"
	"errors"
	"time"

	"github.com/Coombszy/scrutiny/internal/domain"
)

var (
	// ErrNotFound is returned when the PR does not exist or is not visible.
	ErrNotFound = errors.New("not found")
	// ErrOffline is returned when the code host is unreachable and there is
	// no cached copy to serve instead.
	ErrOffline = errors.New("code host unreachable")
)

// Status is the code host connection's health as of the last request.
type Status struct {
	Online bool
	// RateLimit is the most constrained limit seen; Limit is zero before
	// any response has reported one.
	RateLimit RateLimit
}

// RateLimit is an API rate limit's state.
type RateLimit struct {
	Remaining, Limit int
	Reset            time.Time
}

// Low reports whether less than 10% of the limit remains.
func (r RateLimit) Low() bool { return r.Limit > 0 && r.Remaining*10 < r.Limit }

// Dashboard is the set of PR lists shown on the Dashboard.
type Dashboard struct {
	ReviewRequested []domain.PRSummary
	Authored        []domain.PRSummary
}

// NewThread is a Review Thread to add to a Pending Review. StartLine is zero
// for a single-line thread.
type NewThread struct {
	Path            string
	Line, StartLine int
	Side, StartSide domain.DiffSide
	Body            string
}

// CodeHost reads Pull Requests and their review state, and writes review
// activity. Writes return only errors: callers re-read ReviewState, keeping
// the code host the single source of truth.
type CodeHost interface {
	PullRequest(ctx context.Context, ref domain.PRRef) (domain.PullRequest, error)
	// Commits lists the PR's commits, oldest first.
	Commits(ctx context.Context, ref domain.PRRef) ([]domain.Commit, error)
	// Dashboard lists the open PRs awaiting the viewer's review and those
	// they authored.
	Dashboard(ctx context.Context) (Dashboard, error)

	// Checks reads the Checks on the PR's head commit and its MergeState.
	Checks(ctx context.Context, ref domain.PRRef) (domain.Checks, error)
	// Status reports reachability and rate limits. Reads are served from
	// cache while the code host is unreachable.
	Status() Status

	// ReviewState reads the PR's Review Threads, Conversation Comments,
	// Reactions, the viewer's Pending Review and Viewed files.
	ReviewState(ctx context.Context, ref domain.PRRef) (domain.ReviewState, error)
	// Forget drops whatever the code host adapter has cached locally for
	// the PR. Nothing on the code host itself changes.
	Forget(ctx context.Context, ref domain.PRRef) error
	// StartReview creates a Pending Review anchored to commit.
	StartReview(ctx context.Context, prID string, commit domain.SHA) (reviewID string, err error)
	// AddThread adds a Review Thread to a Pending Review.
	AddThread(ctx context.Context, reviewID string, t NewThread) error
	// Reply adds a Review Comment to a thread, within a Pending Review.
	Reply(ctx context.Context, reviewID, threadID, body string) error
	SetThreadResolved(ctx context.Context, threadID string, resolved bool) error
	// AddConversationComment posts a Conversation Comment immediately.
	AddConversationComment(ctx context.Context, prID, body string) error
	// SetReaction adds or removes the viewer's Reaction on a comment or PR.
	SetReaction(ctx context.Context, subjectID string, r domain.Reaction, on bool) error
	// SubmitReview submits the Pending Review, or when reviewID is empty
	// creates and submits a review at commit in one step.
	SubmitReview(ctx context.Context, prID, reviewID string, commit domain.SHA, event domain.ReviewEvent, body string) error
	SetViewed(ctx context.Context, prID, path string, viewed bool) error
}
