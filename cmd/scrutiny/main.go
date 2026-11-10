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
	"github.com/Coombszy/scrutiny/internal/highlight/treesitter"
	"github.com/Coombszy/scrutiny/internal/live"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/navigator/lsp"
	"github.com/Coombszy/scrutiny/internal/repostore/gitcli"
	"github.com/Coombszy/scrutiny/internal/review"
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
		return errors.New("usage: scrutiny serve [-port N]")
	}
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fl.Int("port", 0, "port to listen on, overriding the config file (127.0.0.1 only)")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	return serve(*port)
}

func serve(portOverride int) error {
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
	host, err := github.NewFromGH(db)
	if err != nil {
		return err
	}
	nav := lsp.NewManager(languageServers(dirs), 10*time.Minute)
	defer nav.Close()
	repos := gitcli.New(filepath.Join(dirs.State, "repos"), filepath.Join(dirs.State, "worktrees"), gitcli.HTTPSCloneURL, gitcli.GHCredentialConfig("github.com")...)
	svc := review.NewService(review.Deps{
		CodeHost:    host,
		Repos:       repos,
		Highlighter: treesitter.New(),
		Cache:       db,
		Settings:    settings,
		Navigator:   nav,
	})

	hub := live.NewHub()
	nav.OnIndexing(func(ws navigator.Workspace, indexing bool) {
		if ref, ok := svc.WorkspacePR(ws); ok {
			hub.Publish(ref, review.Update{Indexing: &indexing})
		}
	})
	poller := &live.Poller{Source: svc, Hub: hub, Interval: 30 * time.Second, Now: time.Now}
	go poller.Run(ctx)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           web.NewServer(web.NewCSRFToken(), svc, settings, hub).Handler(),
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

// languageServers finds a Language Server for each supported language:
// first among those Scrutiny installed, then on $PATH.
func languageServers(dirs xdg.Dirs) map[string][]string {
	servers := map[string][]string{}
	for lang, bin := range map[string]string{"go": "gopls"} {
		installed := filepath.Join(dirs.Data, "servers", "bin", bin)
		if _, err := os.Stat(installed); err == nil {
			servers[lang] = []string{installed}
		} else if path, err := exec.LookPath(bin); err == nil {
			servers[lang] = []string{path}
		}
	}
	return servers
}
