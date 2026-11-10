// Package review holds the application services for reviewing a Pull
// Request, composed from the ports.
package review

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight"
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
	Now         func() time.Time
}

// Service loads PRs and their diffs.
type Service struct {
	Deps
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
	Files      []FileEntry
}

// FileEntry is a changed file as listed on the PR page.
type FileEntry struct {
	domain.FileChange
	Collapse Collapse
}

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
	return PRPage{PR: pr, Commits: commits, Comparison: cmp, Files: files}, nil
}

// mergeBase returns the merge base of the PR's head with its base branch,
// fetching only when the head is not yet available locally. Merge bases are
// cached per head commit, since moving the base branch forward cannot
// change them.
func (s *Service) mergeBase(ctx context.Context, pr domain.PullRequest) (domain.SHA, error) {
	key := fmt.Sprintf("mergebase:%s@%s", pr.Ref, pr.HeadSHA)
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
	baseTip, err := s.Repos.FetchPR(ctx, pr.Ref, pr.BaseBranch)
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
