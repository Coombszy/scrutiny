// Command fakeserve serves the Scrutiny UI over in-memory fakes, with a demo
// pull request, for trying the UI in a browser without touching GitHub.
// Writes (comments, reviews, Viewed) go to the fakes and are lost on exit.
//
//	go run ./cmd/fakeserve [-addr 127.0.0.1:7980]
//
// The demo PR is at /github.com/demo/app/pull/7. Its a.go has a resolved and
// an unresolved Review Thread; b.go is marked Viewed and has a resolved one.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight/treesitter"
	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/live"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/servers"
	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/web"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:7980", "address to listen on")
	flag.Parse()

	host, repos, ref := demo()
	cfg := settings.NewMemory(settings.Defaults())
	svc := review.NewService(review.Deps{CodeHost: host, Repos: repos, Highlighter: treesitter.New(), Cache: cache.NewMemory(), Settings: cfg})
	handler := web.NewServer(web.Deps{
		CSRFToken: "fake", Review: svc, Settings: cfg, Hub: live.NewHub(),
		Servers: servers.New(installer.NewFake(), exec.LookPath),
	}).Handler()
	log.Printf("serving the fake demo PR at http://%s%s", *addr, ref.Path())
	log.Fatal(http.ListenAndServe(*addr, handler))
}

// demo builds fakes holding one open PR of two commits that each modify a
// 200-line Go file in two places, far enough apart to leave gaps to expand.
func demo() (*codehost.Fake, *repostore.Fake, domain.PRRef) {
	ref := domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: "demo", Name: "app"}, Number: 7}
	sha := func(c string) domain.SHA { return domain.SHA(strings.Repeat(c, 40)) }
	tip, base, c1, c2 := sha("1"), sha("2"), sha("3"), sha("4")
	now := time.Now()

	host, repos := codehost.NewFake(), repostore.NewFake()
	host.PRs[ref] = domain.PullRequest{
		Ref: ref, Title: "Demo pull request", Author: "sam", State: domain.Open,
		BaseBranch: "main", HeadBranch: "demo", BaseSHA: tip, HeadSHA: c2,
		Body: "A pull request served by fakeserve.", UpdatedAt: now,
	}
	host.CommitsOf[ref] = []domain.Commit{
		{SHA: c1, Headline: "Change a.go", Author: "sam", AuthoredAt: now.Add(-2 * time.Hour)},
		{SHA: c2, Headline: "Change b.go", Author: "sam", AuthoredAt: now.Add(-time.Hour)},
	}
	st := host.State(ref)
	st.Viewer = "you"
	st.Viewed = map[string]domain.ViewedState{"b.go": domain.Viewed}
	comment := func(id, author, text string) domain.Comment {
		return domain.Comment{ID: id, Author: author, Body: text, BodyHTML: "<p>" + text + "</p>", CreatedAt: now.Add(-30 * time.Minute)}
	}
	st.Threads = []domain.ReviewThread{
		{ID: "T1", Path: "a.go", Line: 50, Side: domain.Right, Resolved: true, ViewerCanUnresolve: true,
			Comments: []domain.Comment{comment("C1", "pat", "Resolved: this was fixed.")}},
		{ID: "T2", Path: "a.go", Line: 121, Side: domain.Right, ViewerCanResolve: true,
			Comments: []domain.Comment{comment("C2", "pat", "Is this needed?"), comment("C3", "sam", "Yes, see the issue.")}},
		{ID: "T3", Path: "b.go", Line: 50, Side: domain.Right, Resolved: true, ViewerCanUnresolve: true,
			Comments: []domain.Comment{comment("C4", "pat", "Resolved, in a Viewed file.")}},
	}
	st.Conversation = []domain.Comment{comment("C5", "pat", "Thanks, looking now.")}

	repos.Remotes[ref] = repostore.FakeRemote{BaseTip: tip, Commits: []domain.SHA{c1, c2}}
	repos.MergeBases[repostore.Pair{From: tip, To: c2}] = base
	repos.MergeBases[repostore.Pair{From: tip, To: c1}] = base
	files := []string{"a.go", "b.go"}
	repos.Blobs[base], repos.Blobs[c1], repos.Blobs[c2] = map[string]string{}, map[string]string{}, map[string]string{}
	full, onlyA := map[string]domain.FileDiff{}, map[string]domain.FileDiff{}
	var changes []domain.FileChange
	for _, p := range files {
		old, d := changed(p)
		repos.Blobs[base][p] = strings.Join(old, "\n") + "\n"
		repos.Blobs[c2][p] = strings.Join(d.newLines, "\n") + "\n"
		if p == "a.go" {
			repos.Blobs[c1][p] = repos.Blobs[c2][p]
			onlyA[p] = d.diff
		} else {
			repos.Blobs[c1][p] = repos.Blobs[base][p]
		}
		full[p] = d.diff
		changes = append(changes, d.diff.FileChange)
	}
	repos.ChangeSets[repostore.Pair{From: base, To: c2}] = changes
	repos.Diffs[repostore.Pair{From: base, To: c2}] = full
	repos.ChangeSets[repostore.Pair{From: base, To: c1}] = changes[:1]
	repos.Diffs[repostore.Pair{From: base, To: c1}] = onlyA
	repos.ChangeSets[repostore.Pair{From: c1, To: c2}] = changes[1:]
	repos.Diffs[repostore.Pair{From: c1, To: c2}] = map[string]domain.FileDiff{"b.go": full["b.go"]}
	return host, repos, ref
}

type fileDiff struct {
	newLines []string
	diff     domain.FileDiff
}

// changed returns path's old content and its diff: line 50 is edited and a
// line is added after line 120.
func changed(path string) ([]string, fileDiff) {
	var old []string
	for i := 1; i <= 200; i++ {
		old = append(old, fmt.Sprintf("var line%d = %d", i, i))
	}
	edited := "var line50 = 5000 // edited"
	added := "var extra = true"
	newLines := append(append(append([]string{}, old[:49]...), edited), old[50:120]...)
	newLines = append(append(newLines, added), old[120:]...)

	ctx := func(o, n int) domain.Line { return domain.Line{Kind: domain.Context, OldNo: o, NewNo: n, Text: old[o-1]} }
	change := domain.FileChange{Path: path, OldPath: path, Status: domain.Modified, Additions: 2, Deletions: 1}
	return old, fileDiff{newLines: newLines, diff: domain.FileDiff{FileChange: change, Hunks: []domain.Hunk{
		{OldStart: 47, OldLines: 7, NewStart: 47, NewLines: 7, Lines: []domain.Line{
			ctx(47, 47), ctx(48, 48), ctx(49, 49),
			{Kind: domain.Delete, OldNo: 50, Text: old[49]}, {Kind: domain.Add, NewNo: 50, Text: edited},
			ctx(51, 51), ctx(52, 52), ctx(53, 53),
		}},
		{OldStart: 118, OldLines: 6, NewStart: 118, NewLines: 7, Lines: []domain.Line{
			ctx(118, 118), ctx(119, 119), ctx(120, 120),
			{Kind: domain.Add, NewNo: 121, Text: added},
			ctx(121, 122), ctx(122, 123), ctx(123, 124),
		}},
	}}}
}
