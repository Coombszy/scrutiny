// Package codehost is the CodeHost port: Pull Requests and review state on
// the code host, which is the source of truth for review state.
package codehost

import (
	"context"
	"errors"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// ErrNotFound is returned when the PR does not exist or is not visible.
var ErrNotFound = errors.New("not found")

// Dashboard is the set of PR lists shown on the Dashboard.
type Dashboard struct {
	ReviewRequested []domain.PRSummary
	Authored        []domain.PRSummary
}

// CodeHost reads Pull Requests.
type CodeHost interface {
	PullRequest(ctx context.Context, ref domain.PRRef) (domain.PullRequest, error)
	// Commits lists the PR's commits, oldest first.
	Commits(ctx context.Context, ref domain.PRRef) ([]domain.Commit, error)
	// Dashboard lists the open PRs awaiting the viewer's review and those
	// they authored.
	Dashboard(ctx context.Context) (Dashboard, error)
}
