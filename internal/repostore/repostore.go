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
	// FetchPR makes the PR's head and its base branch available locally.
	FetchPR(ctx context.Context, pr domain.PRRef, baseBranch string) error
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
}
