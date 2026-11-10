package gitcli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/repostore/gitcli"
)

// remote is a throwaway local repository standing in for GitHub.
type remote struct {
	t   *testing.T
	dir string
}

func newRemote(t *testing.T) *remote {
	t.Helper()
	r := &remote{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	return r
}

func (r *remote) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *remote) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *remote) commit(msg string) domain.SHA {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "--allow-empty", "-m", msg)
	return domain.SHA(r.git("rev-parse", "HEAD"))
}

// openPR points refs/pull/<n>/head at sha, as GitHub does.
func (r *remote) openPR(n int, sha domain.SHA) {
	r.git("update-ref", "refs/pull/"+strconv.Itoa(n)+"/head", string(sha))
}

var testRepo = domain.Repo{Host: "github.com", Owner: "o", Name: "r"}

// storeFor returns a Store whose clone URL for testRepo is the remote.
func storeFor(t *testing.T, r *remote) *gitcli.Store {
	t.Helper()
	return gitcli.New(t.TempDir(), t.TempDir(), func(domain.Repo) string { return r.dir })
}
