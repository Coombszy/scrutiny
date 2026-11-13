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
	// ErrUnauthorized is returned when the code host rejects the
	// credentials, e.g. an expired or revoked `gh auth` token.
	ErrUnauthorized = errors.New("code host rejected the credentials")
)

// Status is the code host connection's health as of the last request.
type Status struct {
	Online bool
	// Unauthorized is set while the code host rejects the credentials.
	Unauthorized bool
	// RateLimit is the most constrained limit seen; Limit is zero before
	// any response has reported one.
	RateLimit RateLimit
}

// RateLimit is an API rate limit's state.
type RateLimit struct {
	Remaining, Limit int
	Reset            time.Time
	Resource         string // which limit, e.g. "core", "graphql" or "search"
}

// Low reports whether less than 10% of the limit remains.
func (r RateLimit) Low() bool { return r.Limit > 0 && r.Remaining*10 < r.Limit }

// Dashboard is the set of PR lists shown on the Dashboard.
type Dashboard struct {
	ReviewRequested []domain.PRSummary
	Authored        []domain.PRSummary
}

// NewThread is a Review Thread to add to a Pending Review or post as a
// Single Comment. StartLine is zero
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
	// PostThread posts a Review Thread on commit straight away, as a Single
	// Comment.
	PostThread(ctx context.Context, ref domain.PRRef, commit domain.SHA, t NewThread) error
	// Reply adds a Review Comment to a thread within a Pending Review, or
	// when reviewID is empty posts it as a Single Comment.
	Reply(ctx context.Context, reviewID, threadID, body string) error
	SetThreadResolved(ctx context.Context, threadID string, resolved bool) error
	// EditReviewComment replaces a Review Comment's body, pending or not.
	EditReviewComment(ctx context.Context, commentID, body string) error
	// DeleteReviewComment deletes a Review Comment; a thread left with no
	// comments goes with it.
	DeleteReviewComment(ctx context.Context, commentID string) error
	// AddConversationComment posts a Conversation Comment immediately.
	AddConversationComment(ctx context.Context, prID, body string) error
	// EditConversationComment replaces a Conversation Comment's body.
	EditConversationComment(ctx context.Context, commentID, body string) error
	DeleteConversationComment(ctx context.Context, commentID string) error
	// SetReaction adds or removes the viewer's Reaction on a comment or PR.
	SetReaction(ctx context.Context, subjectID string, r domain.Reaction, on bool) error
	// DiscardReview deletes a Pending Review and its comments.
	DiscardReview(ctx context.Context, reviewID string) error
	// SubmitReview submits the Pending Review, or when reviewID is empty
	// creates and submits a review at commit in one step.
	SubmitReview(ctx context.Context, prID, reviewID string, commit domain.SHA, event domain.ReviewEvent, body string) error
	SetViewed(ctx context.Context, prID, path string, viewed bool) error
	// RenderMarkdown renders a comment body as the code host would show it,
	// sanitised, with references resolved against the PR's repo.
	RenderMarkdown(ctx context.Context, repo domain.Repo, body string) (string, error)
}
