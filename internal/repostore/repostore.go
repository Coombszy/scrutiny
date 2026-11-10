// Package repostore is the RepoStore port: local git data for repositories.
package repostore

import (
	"context"
	"errors"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// ErrNotFound is returned when a commit or path does not exist.
var ErrNotFound = errors.New("not found")

// Store gives access to repository contents at specific commits.
type Store interface {
	// FetchPR makes the PR's head and its base branch available locally,
	// returning the base branch's tip. When the base branch no longer exists
	// (e.g. a merged Stack), baseSHA is fetched and returned instead.
	FetchPR(ctx context.Context, pr domain.PRRef, baseBranch string, baseSHA domain.SHA) (baseTip domain.SHA, err error)
	// HasCommit reports whether sha is available locally.
	HasCommit(ctx context.Context, repo domain.Repo, sha domain.SHA) (bool, error)
	// MergeBase returns the best common ancestor of a and b.
	MergeBase(ctx context.Context, repo domain.Repo, a, b domain.SHA) (domain.SHA, error)
	// Changes lists the files that differ between from and to.
	Changes(ctx context.Context, repo domain.Repo, from, to domain.SHA) ([]domain.FileChange, error)
	// FileDiff returns the hunks for one change listed by Changes.
	FileDiff(ctx context.Context, repo domain.Repo, from, to domain.SHA, change domain.FileChange) (domain.FileDiff, error)
	// Blob returns the contents of path at sha.
	Blob(ctx context.Context, repo domain.Repo, sha domain.SHA, path string) ([]byte, error)
	// ListFiles returns every file path at sha.
	ListFiles(ctx context.Context, repo domain.Repo, sha domain.SHA) ([]string, error)
	// Worktree checks sha out into the PR's own worktree, creating it or
	// moving it as needed, and returns its directory.
	Worktree(ctx context.Context, pr domain.PRRef, sha domain.SHA) (dir string, err error)
	// RemoveWorktree deletes the PR's worktree, if it has one.
	RemoveWorktree(ctx context.Context, pr domain.PRRef) error
	// Attribute returns the gitattributes value of attr for each path, read
	// from .gitattributes at sha: "set", "unset", "unspecified" or the value.
	Attribute(ctx context.Context, repo domain.Repo, sha domain.SHA, attr string, paths []string) (map[string]string, error)
}
