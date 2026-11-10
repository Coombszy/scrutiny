package review

import (
	"context"
	"path"

	"github.com/Coombszy/scrutiny/internal/domain"
)

// Collapse is whether a file's diff starts collapsed, and why.
type Collapse string

const (
	Expanded           Collapse = ""
	CollapsedLarge     Collapse = "large"
	CollapsedGenerated Collapse = "generated"
)

// lockfiles are treated as generated unless .gitattributes says otherwise.
var lockfiles = map[string]bool{
	"go.sum": true, "package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"bun.lockb": true, "bun.lock": true, "Cargo.lock": true, "poetry.lock": true, "uv.lock": true,
	"Pipfile.lock": true, "composer.lock": true, "Gemfile.lock": true, "flake.lock": true,
	"mix.lock": true, "pubspec.lock": true, "Podfile.lock": true, "packages.lock.json": true,
}

// collapse decides which files start collapsed: generated files
// (linguist-generated, or known lockfiles) and those over threshold changed
// lines.
func (s *Service) collapse(ctx context.Context, repo domain.Repo, head domain.SHA, changes []domain.FileChange) ([]FileEntry, error) {
	cfg, err := s.Settings.Load()
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(changes))
	for i, c := range changes {
		paths[i] = c.Path
	}
	generated, err := s.Repos.Attribute(ctx, repo, head, "linguist-generated", paths)
	if err != nil {
		return nil, err
	}
	files := make([]FileEntry, len(changes))
	for i, c := range changes {
		files[i] = FileEntry{FileChange: c}
		switch attr := generated[c.Path]; {
		case attr == "set" || attr == "true",
			attr == "unspecified" && lockfiles[path.Base(c.Path)]:
			files[i].Collapse = CollapsedGenerated
		case c.ChangedLines() > cfg.Diff.CollapseThreshold:
			files[i].Collapse = CollapsedLarge
		}
	}
	return files, nil
}
