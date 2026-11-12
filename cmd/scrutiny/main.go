// Command scrutiny serves a local GitHub pull request review UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache/sqlite"
	"github.com/Coombszy/scrutiny/internal/codehost/github"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/highlight/treesitter"
	"github.com/Coombszy/scrutiny/internal/installer/mason"
	"github.com/Coombszy/scrutiny/internal/live"
	"github.com/Coombszy/scrutiny/internal/metrics"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/navigator/lsp"
	"github.com/Coombszy/scrutiny/internal/release"
	"github.com/Coombszy/scrutiny/internal/release/goproxy"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/repostore/gitcli"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/servers"
	"github.com/Coombszy/scrutiny/internal/settings/tomlfile"
	"github.com/Coombszy/scrutiny/internal/web"
	"github.com/Coombszy/scrutiny/internal/xdg"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "scrutiny:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] != "serve" {
		return errors.New("usage: scrutiny serve [-port N] [-dev]")
	}
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fl.Int("port", 0, "port to listen on, overriding the config file (127.0.0.1 only)")
	dev := fl.Bool("dev", false, "allow inline scripts, for a live-reload proxy (just run-live)")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	return serve(*port, *dev)
}

func serve(portOverride int, dev bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dirs, err := xdg.Resolve()
	if err != nil {
		return err
	}
	settings := tomlfile.New(filepath.Join(dirs.Config, "config.toml"))
	cfg, err := settings.Load()
	if err != nil {
		return err
	}
	port := cfg.Port
	if portOverride != 0 {
		port = portOverride
	}
	db, err := sqlite.Open(filepath.Join(dirs.State, "cache.db"))
	if err != nil {
		return err
	}
	defer db.Close()
	counts := metrics.NewCollector(db, time.Now)
	go counts.Run(ctx, 15*time.Second)
	defer counts.Flush(context.Background()) // before the database closes
	host, err := github.NewFromGH(db, func(base http.RoundTripper) http.RoundTripper {
		return metrics.Transport(base, counts)
	})
	if err != nil {
		return err
	}
	serverDir := filepath.Join(dirs.Data, "servers")
	languageServers := servers.New(mason.New(mason.Options{Dir: serverDir}), exec.LookPath)
	nav := lsp.NewManager(languageServers.Command, 10*time.Minute)
	languageServers.Logs = nav
	defer nav.Close()
	repos := gitcli.New(filepath.Join(dirs.State, "repos"), filepath.Join(dirs.State, "worktrees"), gitcli.HTTPSCloneURL, gitcli.GHCredentialConfig("github.com")...)
	svc := review.NewService(review.Deps{
		CodeHost:    host,
		Repos:       countedRepos{Store: repos, counts: counts},
		Highlighter: treesitter.New(),
		Cache:       db,
		Settings:    settings,
		Navigator:   nav,
	})

	hub := live.NewHub()
	// Push Language Server state to open pages: the PR's warm-up and each
	// server's phases.
	publishLanguages := func(ref domain.PRRef) {
		langs := svc.LanguageStatus(ctx, ref)
		if langs == nil {
			langs = []review.LanguageStatus{}
		}
		hub.Publish(ref, review.Update{Languages: langs})
	}
	svc.OnLanguageStatus = publishLanguages
	nav.OnState(func(ws navigator.Workspace, _ navigator.ServerState) {
		if ref, ok := svc.WorkspacePR(ws); ok {
			publishLanguages(ref)
		}
	})
	checker := &release.Checker{Source: goproxy.New(os.Getenv("GOPROXY"), release.Module), Current: release.BuildVersion()}
	go checker.Run(ctx, 10*time.Minute)
	poller := &live.Poller{Source: svc, Hub: hub, Interval: 30 * time.Second, Now: time.Now}
	go poller.Run(ctx)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler: web.NewServer(web.Deps{
			CSRFToken: web.NewCSRFToken(), Review: svc, Settings: settings, Hub: hub,
			Servers: languageServers, ThemesDir: filepath.Join(dirs.Config, "themes"), Dev: dev,
			Metrics: counts, Release: checker,
		}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		// Requests share ctx so that open event streams end on shutdown.
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	slog.Info("scrutiny listening", "url", "http://"+ln.Addr().String())

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// countedRepos counts git fetches and their failures.
type countedRepos struct {
	repostore.Store
	counts *metrics.Collector
}

func (r countedRepos) FetchPR(ctx context.Context, pr domain.PRRef, baseBranch string, baseSHA domain.SHA) (domain.SHA, error) {
	r.counts.Inc(metrics.GitFetches)
	sha, err := r.Store.FetchPR(ctx, pr, baseBranch, baseSHA)
	if err != nil {
		r.counts.Inc(metrics.GitFetchErrors)
	}
	return sha, err
}
