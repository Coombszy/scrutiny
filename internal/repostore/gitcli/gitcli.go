// Package gitcli is the RepoStore adapter that shells out to the git binary,
// keeping one bare clone per repository.
package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore"
)

// Store keeps bare clones under root, at <root>/<host>/<owner>/<repo>.git.
type Store struct {
	root      string
	cloneURL  func(domain.Repo) string
	gitConfig []string

	mu    sync.Mutex
	locks map[domain.Repo]*sync.Mutex
}

var _ repostore.Store = (*Store)(nil)

// New returns a Store rooted at root. cloneURL maps a repository to the URL
// it is fetched from, and each gitConfig entry ("key=value") is passed to
// every git invocation with -c.
func New(root string, cloneURL func(domain.Repo) string, gitConfig ...string) *Store {
	return &Store{root: root, cloneURL: cloneURL, gitConfig: gitConfig, locks: map[domain.Repo]*sync.Mutex{}}
}

// GHCredentialConfig makes git authenticate to host through `gh auth`,
// replacing any other credential helper configured for it.
func GHCredentialConfig(host string) []string {
	key := "credential.https://" + host + ".helper"
	return []string{key + "=", key + "=!gh auth git-credential"}
}

// HTTPSCloneURL fetches over HTTPS, e.g. https://github.com/owner/repo.git.
func HTTPSCloneURL(r domain.Repo) string {
	return "https://" + r.Host + "/" + r.Owner + "/" + r.Name + ".git"
}

func (s *Store) dir(r domain.Repo) string {
	return filepath.Join(s.root, r.Host, r.Owner, r.Name+".git")
}

// lock serialises fetches per repository.
func (s *Store) lock(r domain.Repo) func() {
	s.mu.Lock()
	l, ok := s.locks[r]
	if !ok {
		l = &sync.Mutex{}
		s.locks[r] = l
	}
	s.mu.Unlock()
	l.Lock()
	return l.Unlock
}

func (s *Store) FetchPR(ctx context.Context, pr domain.PRRef, baseBranch string) (domain.SHA, error) {
	defer s.lock(pr.Repo)()
	dir := s.dir(pr.Repo)
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if _, err := s.git(ctx, pr.Repo, "init", "-q", "--bare"); err != nil {
			return "", err
		}
	}
	n := strconv.Itoa(pr.Number)
	baseRef := "refs/heads/" + baseBranch
	if _, err := s.git(ctx, pr.Repo, "fetch", "-q", "--no-tags", "--force", s.cloneURL(pr.Repo),
		"+"+baseRef+":"+baseRef,
		"+refs/pull/"+n+"/head:refs/pull/"+n+"/head"); err != nil {
		return "", err
	}
	out, err := s.git(ctx, pr.Repo, "rev-parse", "--verify", baseRef+"^{commit}")
	if err != nil {
		return "", err
	}
	return domain.SHA(strings.TrimSpace(string(out))), nil
}

func (s *Store) HasCommit(ctx context.Context, repo domain.Repo, sha domain.SHA) (bool, error) {
	if err := checkRevs(sha); err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(s.dir(repo), "HEAD")); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	_, err := s.git(ctx, repo, "cat-file", "-e", string(sha)+"^{commit}")
	return err == nil, nil
}

func (s *Store) Attribute(ctx context.Context, repo domain.Repo, sha domain.SHA, attr string, paths []string) (map[string]string, error) {
	values := make(map[string]string, len(paths))
	if len(paths) == 0 {
		return values, nil
	}
	stdin := strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := s.gitWithInput(ctx, repo, stdin, "check-attr", "-z", "--stdin", "--source="+string(sha), attr)
	if err != nil {
		return nil, err
	}
	tok := splitNUL(out)
	for i := 0; i+2 < len(tok); i += 3 {
		values[tok[i]] = tok[i+2]
	}
	return values, nil
}

func (s *Store) MergeBase(ctx context.Context, repo domain.Repo, a, b domain.SHA) (domain.SHA, error) {
	if err := checkRevs(a, b); err != nil {
		return "", err
	}
	out, err := s.git(ctx, repo, "merge-base", string(a), string(b))
	if err != nil {
		return "", err
	}
	return domain.SHA(strings.TrimSpace(string(out))), nil
}

func (s *Store) Blob(ctx context.Context, repo domain.Repo, sha domain.SHA, path string) ([]byte, error) {
	if err := checkRevs(sha); err != nil {
		return nil, err
	}
	out, err := s.git(ctx, repo, "cat-file", "blob", string(sha)+":"+path)
	if err != nil {
		return nil, fmt.Errorf("%s:%s: %w", sha.Short(), path, repostore.ErrNotFound)
	}
	return out, nil
}

func (s *Store) ListFiles(ctx context.Context, repo domain.Repo, sha domain.SHA) ([]string, error) {
	if err := checkRevs(sha); err != nil {
		return nil, err
	}
	out, err := s.git(ctx, repo, "ls-tree", "-r", "-z", "--name-only", string(sha))
	if err != nil {
		return nil, err
	}
	return splitNUL(out), nil
}

func (s *Store) Changes(ctx context.Context, repo domain.Repo, from, to domain.SHA) ([]domain.FileChange, error) {
	if err := checkRevs(from, to); err != nil {
		return nil, err
	}
	out, err := s.git(ctx, repo, "diff", "--no-ext-diff", "--find-renames", "--raw", "--numstat", "-z", string(from), string(to))
	if err != nil {
		return nil, err
	}
	return parseRawNumstat(splitNUL(out))
}

// parseRawNumstat parses `git diff --raw --numstat -z`, which prints every
// raw record followed by every numstat record, in the same order.
func parseRawNumstat(tok []string) ([]domain.FileChange, error) {
	var changes []domain.FileChange
	i := 0
	for i < len(tok) && strings.HasPrefix(tok[i], ":") {
		fields := strings.Fields(tok[i])
		if len(fields) < 5 || i+1 >= len(tok) {
			return nil, fmt.Errorf("malformed raw diff record %q", tok[i])
		}
		c := domain.FileChange{Path: tok[i+1], OldPath: tok[i+1]}
		i += 2
		switch fields[4][0] {
		case 'A':
			c.Status = domain.Added
		case 'D':
			c.Status = domain.Deleted
		case 'R', 'C':
			if i >= len(tok) {
				return nil, fmt.Errorf("malformed rename record %q", fields[4])
			}
			c.Status = domain.Renamed
			c.Path = tok[i]
			i++
		default:
			c.Status = domain.Modified
		}
		changes = append(changes, c)
	}
	for n := range changes {
		if i >= len(tok) {
			return nil, errors.New("numstat records missing")
		}
		fields := strings.SplitN(tok[i], "\t", 3)
		if len(fields) != 3 {
			return nil, fmt.Errorf("malformed numstat record %q", tok[i])
		}
		i++
		if fields[2] == "" { // a rename: the old and new paths follow
			i += 2
		}
		if fields[0] == "-" {
			changes[n].Binary = true
			continue
		}
		changes[n].Additions, _ = strconv.Atoi(fields[0])
		changes[n].Deletions, _ = strconv.Atoi(fields[1])
	}
	return changes, nil
}

func (s *Store) FileDiff(ctx context.Context, repo domain.Repo, from, to domain.SHA, change domain.FileChange) (domain.FileDiff, error) {
	if err := checkRevs(from, to); err != nil {
		return domain.FileDiff{}, err
	}
	args := []string{"diff", "--no-ext-diff", "--no-color", "--find-renames", string(from), string(to), "--", change.OldPath}
	if change.Path != change.OldPath {
		args = append(args, change.Path)
	}
	out, err := s.git(ctx, repo, args...)
	if err != nil {
		return domain.FileDiff{}, err
	}
	hunks, err := parseHunks(string(out))
	if err != nil {
		return domain.FileDiff{}, fmt.Errorf("%s: %w", change.Path, err)
	}
	return domain.FileDiff{FileChange: change, Hunks: hunks}, nil
}

// git runs git inside repo's bare clone, ignoring the system config and
// never prompting for credentials.
func (s *Store) git(ctx context.Context, repo domain.Repo, args ...string) ([]byte, error) {
	return s.gitWithInput(ctx, repo, nil, args...)
}

// checkRevs rejects revisions that git would parse as options.
func checkRevs(revs ...domain.SHA) error {
	for _, r := range revs {
		if r == "" || strings.HasPrefix(string(r), "-") {
			return fmt.Errorf("invalid revision %q", r)
		}
	}
	return nil
}

func (s *Store) gitWithInput(ctx context.Context, repo domain.Repo, stdin io.Reader, args ...string) ([]byte, error) {
	full := []string{"-c", "core.quotePath=false"}
	for _, kv := range s.gitConfig {
		full = append(full, "-c", kv)
	}
	cmd := exec.CommandContext(ctx, "git", append(full, args...)...)
	cmd.Dir = s.dir(repo)
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func splitNUL(b []byte) []string {
	parts := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
	if len(parts) == 1 && parts[0] == "" {
		return nil
	}
	return parts
}
