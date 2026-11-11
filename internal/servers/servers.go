// Package servers manages Language Servers for code navigation: which one
// launches for a language, and installing or removing them in the
// background.
package servers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/navigator"
)

// launch holds how to start registry packages whose executable needs
// arguments, or is not the only one they install.
var launch = map[string][]string{
	"gopls":                      {"gopls"},
	"typescript-language-server": {"typescript-language-server", "--stdio"},
	"pyright":                    {"pyright-langserver", "--stdio"},
	"basedpyright":               {"basedpyright-langserver", "--stdio"},
	"yaml-language-server":       {"yaml-language-server", "--stdio"},
	"bash-language-server":       {"bash-language-server", "start"},
	"json-lsp":                   {"vscode-json-language-server", "--stdio"},
	"terraform-ls":               {"terraform-ls", "serve"},
	"intelephense":               {"intelephense", "--stdio"},
	"vtsls":                      {"vtsls", "--stdio"},
}

// pathFallback lists servers used from $PATH when none is installed.
var pathFallback = map[string][]string{
	"go":     {"gopls"},
	"rust":   {"rust-analyzer"},
	"c":      {"clangd"},
	"cpp":    {"clangd"},
	"python": {"pyright-langserver", "--stdio"},
}

// language maps a registry language name to Scrutiny's language id.
func language(name string) string {
	l := strings.ToLower(name)
	switch l {
	case "c++":
		return "cpp"
	case "c#":
		return "csharp"
	case "sh", "zsh", "ksh":
		return "bash"
	}
	return l
}

// LogSource reads a Language Server's recent log by the executable path it
// was launched as.
type LogSource interface {
	Log(executable string) []navigator.LogEntry
}

// Service resolves launch commands and runs install jobs.
type Service struct {
	inst     installer.Installer
	lookPath func(string) (string, error)
	// Logs, if set before use, is where installed servers' logs are read.
	Logs LogSource

	mu   sync.Mutex
	jobs map[string]*Job
}

func New(inst installer.Installer, lookPath func(string) (string, error)) *Service {
	return &Service{inst: inst, lookPath: lookPath, jobs: map[string]*Job{}}
}

// Command returns how to launch a Language Server for lang: an installed
// one first, then a known server on $PATH; nil when there is none.
func (s *Service) Command(lang string) []string {
	installed, _ := s.inst.Installed()
	slices.SortFunc(installed, func(a, b installer.Installed) int { return cmp.Compare(a.Name, b.Name) })
	var guess []string
	for _, in := range installed {
		if !slices.ContainsFunc(in.Languages, func(l string) bool { return language(l) == lang }) {
			continue
		}
		if argv := launch[in.Name]; argv != nil {
			if bin, ok := in.Bin[argv[0]]; ok {
				return append([]string{bin}, argv[1:]...)
			}
		}
		// A package Scrutiny knows nothing about (often a linter) is only a
		// last resort: launched bare, it may not speak LSP over stdio.
		if len(in.Bin) == 1 && guess == nil {
			for _, bin := range in.Bin {
				guess = []string{bin}
			}
		}
	}
	if argv := pathFallback[lang]; argv != nil {
		if bin, err := s.lookPath(argv[0]); err == nil {
			return append([]string{bin}, argv[1:]...)
		}
	}
	return guess
}

// JobState is how a background install is going.
type JobState string

const (
	Installing JobState = "installing"
	Failed     JobState = "failed"
)

// Job is a background install; finished successful jobs are dropped.
type Job struct {
	State   JobState
	Err     string
	Started time.Time
}

// Status is a registry package with its install state on this machine.
type Status struct {
	installer.Package
	Installed *installer.Installed
	Missing   []string // runtimes it needs that are not on $PATH
	Job       *Job
	LogErrors int // errors in its recent log
}

// List returns the registry's Language Servers matching query (by name or
// language), installed ones first.
func (s *Service) List(ctx context.Context, query string) ([]Status, error) {
	pkgs, err := s.inst.Packages(ctx, false)
	if err != nil {
		return nil, err
	}
	installed, err := s.inst.Installed()
	if err != nil {
		return nil, err
	}
	have := map[string]installer.Installed{}
	for _, in := range installed {
		have[in.Name] = in
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Status
	for _, p := range pkgs {
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) &&
			!slices.ContainsFunc(p.Languages, func(l string) bool { return strings.Contains(strings.ToLower(l), q) }) {
			continue
		}
		out = append(out, s.status(p, have))
	}
	slices.SortStableFunc(out, func(a, b Status) int {
		return cmp.Or(cmp.Compare(rank(a), rank(b)), cmp.Compare(a.Name, b.Name))
	})
	return out, nil
}

// rank orders installed and installing packages first, unsupported last.
func rank(s Status) int {
	switch {
	case s.Installed != nil || s.Job != nil:
		return 0
	case s.Unsupported != "":
		return 2
	}
	return 1
}

// Status returns one package's status.
func (s *Service) Status(ctx context.Context, name string) (Status, error) {
	all, err := s.List(ctx, "")
	if err != nil {
		return Status{}, err
	}
	i := slices.IndexFunc(all, func(st Status) bool { return st.Name == name })
	if i < 0 {
		return Status{}, fmt.Errorf("%w: %s", installer.ErrNotFound, name)
	}
	return all[i], nil
}

func (s *Service) status(p installer.Package, have map[string]installer.Installed) Status {
	st := Status{Package: p, Missing: s.inst.Missing(p)}
	if in, ok := have[p.Name]; ok {
		st.Installed = &in
		for _, e := range s.log(in) {
			if e.Level == navigator.LogError {
				st.LogErrors++
			}
		}
	}
	s.mu.Lock()
	if j := s.jobs[p.Name]; j != nil {
		copy := *j
		st.Job = &copy
	}
	s.mu.Unlock()
	return st
}

// Log returns the recent log of installed package name's executables,
// oldest first; none when it is not installed.
func (s *Service) Log(name string) ([]navigator.LogEntry, error) {
	installed, err := s.inst.Installed()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(installed, func(in installer.Installed) bool { return in.Name == name })
	if i < 0 {
		return nil, nil
	}
	return s.log(installed[i]), nil
}

func (s *Service) log(in installer.Installed) []navigator.LogEntry {
	if s.Logs == nil {
		return nil
	}
	var out []navigator.LogEntry
	for _, bin := range in.Bin {
		out = append(out, s.Logs.Log(bin)...)
	}
	slices.SortStableFunc(out, func(a, b navigator.LogEntry) int { return a.Time.Compare(b.Time) })
	return out
}

// Install starts installing name in the background, unless it already is.
func (s *Service) Install(name string) {
	s.mu.Lock()
	if j := s.jobs[name]; j != nil && j.State == Installing {
		s.mu.Unlock()
		return
	}
	job := &Job{State: Installing, Started: time.Now()}
	s.jobs[name] = job
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		err := s.inst.Install(ctx, name)
		s.mu.Lock()
		defer s.mu.Unlock()
		if err != nil {
			job.State, job.Err = Failed, err.Error()
			return
		}
		if s.jobs[name] == job {
			delete(s.jobs, name)
		}
	}()
}

// ErrBusy is returned when removing a package that is being installed.
var ErrBusy = errors.New("still installing")

// Remove uninstalls name.
func (s *Service) Remove(name string) error {
	s.mu.Lock()
	if j := s.jobs[name]; j != nil && j.State == Installing {
		s.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrBusy, name)
	}
	delete(s.jobs, name)
	s.mu.Unlock()
	return s.inst.Remove(name)
}

// Refresh downloads the latest registry.
func (s *Service) Refresh(ctx context.Context) error {
	_, err := s.inst.Packages(ctx, true)
	return err
}
