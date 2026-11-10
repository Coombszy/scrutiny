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
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache/sqlite"
	"github.com/Coombszy/scrutiny/internal/codehost/github"
	"github.com/Coombszy/scrutiny/internal/highlight/treesitter"
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
	repos := gitcli.New(filepath.Join(dirs.State, "repos"), gitcli.HTTPSCloneURL, gitcli.GHCredentialConfig("github.com")...)
	svc := review.NewService(review.Deps{
		CodeHost:    host,
		Repos:       repos,
		Highlighter: treesitter.New(),
		Cache:       db,
		Settings:    settings,
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           web.NewServer(web.NewCSRFToken(), svc, settings).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
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
