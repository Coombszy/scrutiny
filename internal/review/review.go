// Package review holds the application services for reviewing a Pull
// Request, composed from the ports.
package review

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/settings"
)

// ErrInvalidComparison is returned for a commit range that is not a
// contiguous run of the PR's commits.
var ErrInvalidComparison = errors.New("invalid comparison")

// Deps are the ports the Service uses.
type Deps struct {
	CodeHost    codehost.CodeHost
	Repos       repostore.Store
	Highlighter highlight.Highlighter
	Cache       cache.Cache
	Settings    settings.Store
	Navigator   navigator.Navigator // optional: code navigation is off without it
	Now         func() time.Time
}

// Service loads PRs and their diffs.
type Service struct {
	Deps
	ws workspaces
}

func NewService(d Deps) *Service {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Service{Deps: d}
}

// ComparisonRequest selects a Comparison. The zero value is the default
// (merge base to head); otherwise First and Last are the first and last
// commits of a run of the PR's commits.
type ComparisonRequest struct {
	First, Last domain.SHA
}

// Comparison is the pair of commits a diff is shown between.
type Comparison struct {
	Base, Head  domain.SHA
	First, Last domain.SHA // the selected commit range; empty for the default
}

// IsDefault reports whether this is the merge-base-to-head Comparison.
func (c Comparison) IsDefault() bool { return c.First == "" }

// PRPage is everything the PR page shows before any file diff is loaded.
type PRPage struct {
	PR         domain.PullRequest
	Commits    []domain.Commit
	Comparison Comparison
	MergeBase  domain.SHA
	Files      []FileEntry
	Review     domain.ReviewState
	// OtherThreads are Review Threads on files not in this Comparison.
	OtherThreads []domain.ReviewThread
	Checks       domain.Checks
	// SinceLastReview selects the commits pushed since the viewer's last
	// submitted review; nil when there are none or that commit has left
	// the branch.
	SinceLastReview *ComparisonRequest
	// Offline is set when the code host is unreachable and the page is
	// served from the cache; review actions are then unavailable.
	Offline bool
}

// FileEntry is a changed file as listed on the PR page.
type FileEntry struct {
	domain.FileChange
	Collapse Collapse
	Viewed   domain.ViewedState
	// Threads and Unresolved count the file's Review Threads.
	Threads, Unresolved int
}

// CanCommentLeft reports whether new threads may go on the left side: only
// when it is the PR's merge base, which GitHub anchors LEFT lines to.
func (p PRPage) CanCommentLeft() bool { return p.Comparison.Base == p.MergeBase }

// PRPage loads the PR, makes its commits available locally and lists the
// files changed in the requested Comparison.
func (s *Service) PRPage(ctx context.Context, ref domain.PRRef, req ComparisonRequest) (PRPage, error) {
	pr, err := s.CodeHost.PullRequest(ctx, ref)
	if err != nil {
		return PRPage{}, err
	}
	commits, err := s.CodeHost.Commits(ctx, ref)
	if err != nil {
		return PRPage{}, err
	}
	if err := s.Cache.RecordVisit(ctx, cache.Visit{PR: ref, Title: pr.Title, VisitedAt: s.Now()}); err != nil {
		return PRPage{}, err
	}
	if pr.State == domain.Closed || pr.State == domain.Merged {
		if err := s.closeWorkspaces(ctx, ref); err != nil {
			slog.Warn("removing worktree", "pr", ref, "err", err)
		}
	}
	mergeBase, err := s.mergeBase(ctx, pr)
	if err != nil {
		return PRPage{}, err
	}
	cmp, err := resolve(req, commits, mergeBase, pr.HeadSHA)
	if err != nil {
		return PRPage{}, err
	}
	changes, err := s.Repos.Changes(ctx, ref.Repo, cmp.Base, cmp.Head)
	if err != nil {
		return PRPage{}, err
	}
	files, err := s.collapse(ctx, ref.Repo, cmp.Head, changes)
	if err != nil {
		return PRPage{}, err
	}
	snap, err := s.readFresh(ctx, ref, pr.HeadSHA, mergeBase)
	if err != nil {
		return PRPage{}, err
	}
	page := PRPage{
		PR: pr, Commits: commits, Comparison: cmp, MergeBase: mergeBase, Files: files,
		Review: snap.State, Checks: snap.Checks, Offline: snap.Offline,
		SinceLastReview: sinceLastReview(commits, snap.State.LastReviewed),
	}
	inComparison := map[string]bool{}
	for i := range page.Files {
		f := &page.Files[i]
		inComparison[f.Path] = true
		f.Viewed = snap.State.Viewed[f.Path]
		for _, t := range snap.State.Threads {
			if t.Path == f.Path {
				f.Threads++
				if !t.Resolved {
					f.Unresolved++
				}
			}
		}
	}
	for _, t := range snap.State.Threads {
		if !inComparison[t.Path] {
			page.OtherThreads = append(page.OtherThreads, t)
		}
	}
	return page, nil
}

// mergeBase returns the merge base of the PR's head with its base branch,
// fetching only when the head is not yet available locally. Merge bases are
// cached per base branch and head commit, since moving the base branch
// forward cannot change them (a force-pushed base branch can, until the
// head moves).
func (s *Service) mergeBase(ctx context.Context, pr domain.PullRequest) (domain.SHA, error) {
	key := fmt.Sprintf("mergebase:%s:%s@%s", pr.Ref, pr.BaseBranch, pr.HeadSHA)
	have, err := s.Repos.HasCommit(ctx, pr.Ref.Repo, pr.HeadSHA)
	if err != nil {
		return "", err
	}
	if have {
		if e, ok, err := s.Cache.Get(ctx, key); err != nil {
			return "", err
		} else if ok {
			return domain.SHA(e.Value), nil
		}
	}
	baseTip, err := s.Repos.FetchPR(ctx, pr.Ref, pr.BaseBranch, pr.BaseSHA)
	if err != nil {
		return "", err
	}
	mb, err := s.Repos.MergeBase(ctx, pr.Ref.Repo, baseTip, pr.HeadSHA)
	if err != nil {
		return "", err
	}
	return mb, s.Cache.Put(ctx, key, cache.Entry{Value: []byte(mb), FetchedAt: s.Now()})
}

// resolve turns a ComparisonRequest into a Comparison. A run of commits is
// compared from the parent of its first commit, which for the PR's first
// commit is the merge base.
func resolve(req ComparisonRequest, commits []domain.Commit, mergeBase, head domain.SHA) (Comparison, error) {
	if req.First == "" && req.Last == "" {
		return Comparison{Base: mergeBase, Head: head}, nil
	}
	first, last := -1, -1
	for i, c := range commits {
		if c.SHA == req.First {
			first = i
		}
		if c.SHA == req.Last {
			last = i
		}
	}
	if first < 0 || last < 0 || first > last {
		return Comparison{}, fmt.Errorf("%w: %s..%s", ErrInvalidComparison, req.First.Short(), req.Last.Short())
	}
	base := mergeBase
	if first > 0 {
		base = commits[first-1].SHA
	}
	return Comparison{Base: base, Head: req.Last, First: req.First, Last: req.Last}, nil
}

// sinceLastReview is the run of commits after reviewed, if reviewed is
// still on the branch and is not its head.
func sinceLastReview(commits []domain.Commit, reviewed domain.SHA) *ComparisonRequest {
	if reviewed == "" {
		return nil
	}
	for i, c := range commits {
		if c.SHA == reviewed && i+1 < len(commits) {
			return &ComparisonRequest{First: commits[i+1].SHA, Last: commits[len(commits)-1].SHA}
		}
	}
	return nil
}
