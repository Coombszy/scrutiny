// Package mason is the ServerInstaller adapter for the Mason registry
// (mason-org/mason-registry). It installs npm, Go and GitHub release
// packages under one directory, normally $XDG_DATA_HOME/scrutiny/servers:
//
//	registry.json             the cached registry
//	packages/<name>/          each package, with receipt.json
//	bin/<executable>          links to every installed executable
package mason

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/Coombszy/scrutiny/internal/installer"
)

// Options configure an Installer; zero fields get real defaults.
type Options struct {
	Dir          string
	HTTP         *http.Client
	APIBase      string // https://api.github.com
	DownloadBase string // https://github.com
	GOOS, GOARCH string
	// Run runs argv in dir with env; LookPath finds runtimes.
	Run      func(ctx context.Context, dir string, env []string, argv ...string) error
	LookPath func(string) (string, error)
}

// Installer installs Language Servers from the Mason registry.
type Installer struct {
	opts Options
}

var _ installer.Installer = (*Installer)(nil)

func New(opts Options) *Installer {
	if opts.HTTP == nil {
		opts.HTTP = http.DefaultClient
	}
	if opts.APIBase == "" {
		opts.APIBase = "https://api.github.com"
	}
	if opts.DownloadBase == "" {
		opts.DownloadBase = "https://github.com"
	}
	if opts.GOOS == "" {
		opts.GOOS, opts.GOARCH = runtime.GOOS, runtime.GOARCH
	}
	if opts.Run == nil {
		opts.Run = run
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	return &Installer{opts: opts}
}

func run(ctx context.Context, dir string, env []string, argv ...string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	if err != nil {
		tail := strings.TrimSpace(string(out))
		if len(tail) > 2000 {
			tail = "…" + tail[len(tail)-2000:]
		}
		return fmt.Errorf("%s: %w\n%s", strings.Join(argv, " "), err, tail)
	}
	return nil
}

// plan is how to install one package on this machine.
type plan struct {
	pkg   installer.Package
	purl  purl
	spec  spec
	asset *asset            // the GitHub release asset for this platform
	bins  map[string]string // executable name -> path within the package
}

func (i *Installer) Packages(ctx context.Context, refresh bool) ([]installer.Package, error) {
	specs, err := i.loadRegistry(ctx, refresh)
	if err != nil {
		return nil, err
	}
	var pkgs []installer.Package
	for _, s := range specs {
		if slices.Contains(s.Categories, "LSP") {
			pkgs = append(pkgs, i.plan(s).pkg)
		}
	}
	return pkgs, nil
}

// plan works out how to install s here, or why it cannot be.
func (i *Installer) plan(s spec) plan {
	p := plan{spec: s, pkg: installer.Package{
		Name: s.Name, Description: s.Description, Homepage: s.Homepage, Languages: s.Languages,
	}}
	pu, err := parsePURL(s.Source.ID)
	if err != nil {
		p.pkg.Unsupported = err.Error()
		return p
	}
	p.purl, p.pkg.Source, p.pkg.Version = pu, pu.Type, pu.Version
	unsupported := func(format string, args ...any) plan {
		p.pkg.Unsupported = fmt.Sprintf(format, args...)
		return p
	}
	switch pu.Type {
	case "npm":
		p.pkg.Requires = []string{"node", "npm"}
	case "golang":
		p.pkg.Requires = []string{"go"}
	case "github":
		if len(s.Source.Build) > 0 {
			return unsupported("builds from source")
		}
		a, ok := i.assetFor(s.Source.Asset)
		if !ok {
			return unsupported("no release for %s/%s", i.opts.GOOS, i.opts.GOARCH)
		}
		a.File = expand(a.File, pu.Version, nil)
		p.asset = &a
	default:
		return unsupported("installs from %s, which Scrutiny does not support yet", pu.Type)
	}
	p.bins = map[string]string{}
	for name, b := range s.Bin {
		path, err := i.binPath(expand(b, pu.Version, p.asset), pu.Type)
		if err != nil {
			return unsupported("%v", err)
		}
		p.bins[name] = path
	}
	return p
}

// binPath turns a registry bin spec into a path within the package.
func (i *Installer) binPath(b, sourceType string) (string, error) {
	kind, rest, ok := strings.Cut(b, ":")
	if !ok {
		return filepath.FromSlash(b), nil
	}
	switch {
	case kind == "npm" && sourceType == "npm":
		return filepath.Join("node_modules", ".bin", rest), nil
	case kind == "golang" && sourceType == "golang":
		return rest, nil
	case kind == "exec":
		return filepath.FromSlash(rest), nil
	}
	return "", fmt.Errorf("needs the %s runtime to launch", kind)
}

// targets are the Mason asset targets that run here, best first.
func (i *Installer) targets() []string {
	arch := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}[i.opts.GOARCH]
	switch i.opts.GOOS {
	case "linux":
		return []string{"linux_" + arch + "_gnu", "linux_" + arch, "linux_" + arch + "_musl"}
	case "darwin":
		return []string{"darwin_" + arch}
	}
	return nil
}

func (i *Installer) assetFor(raw json.RawMessage) (asset, bool) {
	var assets []asset
	if json.Unmarshal(raw, &assets) != nil {
		var one asset
		if json.Unmarshal(raw, &one) != nil {
			return asset{}, false
		}
		assets = []asset{one}
	}
	for _, want := range i.targets() {
		for _, a := range assets {
			if slices.Contains(a.targets(), want) {
				return a, true
			}
		}
	}
	return asset{}, false
}

// expand fills in the registry's templates: {{version}}, with an optional
// `strip_prefix "v"` filter, and {{source.asset.file|bin}}.
func expand(s, version string, a *asset) string {
	for {
		start := strings.Index(s, "{{")
		end := strings.Index(s, "}}")
		if start < 0 || end < start {
			return s
		}
		expr := strings.TrimSpace(s[start+2 : end])
		var value string
		name, filter, _ := strings.Cut(expr, "|")
		switch strings.TrimSpace(name) {
		case "version":
			value = version
		case "source.asset.file":
			if a != nil {
				value = a.File
			}
		case "source.asset.bin":
			if a != nil {
				value = a.Bin
			}
		}
		if f := strings.Fields(filter); len(f) == 2 && f[0] == "strip_prefix" {
			value = strings.TrimPrefix(value, strings.Trim(f[1], `"`))
		}
		s = s[:start] + value + s[end+2:]
	}
}

func (i *Installer) Missing(p installer.Package) []string {
	var missing []string
	for _, r := range p.Requires {
		if _, err := i.opts.LookPath(r); err != nil {
			missing = append(missing, r)
		}
	}
	return missing
}

func (i *Installer) packageDir(name string) string {
	return filepath.Join(i.opts.Dir, "packages", name)
}
func (i *Installer) binDir() string { return filepath.Join(i.opts.Dir, "bin") }

func (i *Installer) Install(ctx context.Context, name string) error {
	specs, err := i.loadRegistry(ctx, false)
	if err != nil {
		return err
	}
	idx := slices.IndexFunc(specs, func(s spec) bool { return s.Name == name })
	if idx < 0 {
		return fmt.Errorf("%w: %s", installer.ErrNotFound, name)
	}
	p := i.plan(specs[idx])
	if p.pkg.Unsupported != "" {
		return fmt.Errorf("%w: %s %s", installer.ErrUnsupported, name, p.pkg.Unsupported)
	}
	if missing := i.Missing(p.pkg); len(missing) > 0 {
		return fmt.Errorf("%w: %s requires %s", installer.ErrMissingRuntime, name, strings.Join(missing, " and "))
	}
	// Install beside any current version, swapping it in only on success,
	// so a failed update leaves the working one in place.
	if err := os.MkdirAll(filepath.Join(i.opts.Dir, "packages"), 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Join(i.opts.Dir, "packages"), "."+name+".")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := i.fetch(ctx, p, staging); err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	for bin, rel := range p.bins {
		if _, err := os.Stat(filepath.Join(staging, rel)); err != nil {
			return fmt.Errorf("install %s: executable %s missing: %w", name, rel, err)
		}
		if owner := i.linkOwner(bin); owner != "" && owner != name {
			return fmt.Errorf("install %s: executable %s already belongs to %s", name, bin, owner)
		}
	}
	dir := i.packageDir(name)
	old := staging + ".old"
	if err := os.Rename(dir, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(staging, dir); err != nil {
		os.Rename(old, dir)
		return err
	}
	defer os.RemoveAll(old)
	return i.link(p, dir)
}

// linkOwner returns the package that bin/<name> belongs to: "" when there
// is no link, or "?" when it points somewhere Scrutiny did not install.
func (i *Installer) linkOwner(name string) string {
	target, err := os.Readlink(filepath.Join(i.binDir(), name))
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(filepath.Join(i.opts.Dir, "packages"), target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "?"
	}
	pkg, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	return pkg
}

// fetch installs the package's files into dir.
func (i *Installer) fetch(ctx context.Context, p plan, dir string) error {
	switch p.purl.Type {
	case "golang":
		target := p.purl.fullName()
		if p.purl.Subpath != "" {
			target += "/" + p.purl.Subpath
		}
		return i.opts.Run(ctx, dir, append(os.Environ(), "GOBIN="+dir), "go", "install", target+"@"+p.purl.Version)
	case "npm":
		argv := []string{"npm", "install", "--prefix", dir, "--no-save", "--no-audit", "--no-fund", p.purl.fullName() + "@" + p.purl.Version}
		return i.opts.Run(ctx, dir, os.Environ(), append(argv, p.spec.Source.ExtraPackages...)...)
	case "github":
		return i.fetchRelease(ctx, p, dir)
	}
	return fmt.Errorf("%w: %s", installer.ErrUnsupported, p.purl.Type)
}

// receipt records an installed package in its directory.
type receipt struct {
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Languages []string          `json:"languages"`
	Bin       map[string]string `json:"bin"` // name -> path within the package
}

// link makes each executable runnable and links it from bin/, then writes
// the receipt.
func (i *Installer) link(p plan, dir string) error {
	if err := os.MkdirAll(i.binDir(), 0o755); err != nil {
		return err
	}
	for name, rel := range p.bins {
		target := filepath.Join(dir, rel)
		fi, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("install %s: executable %s missing: %w", p.pkg.Name, rel, err)
		}
		if err := os.Chmod(target, fi.Mode()|0o111); err != nil {
			return err
		}
		link := filepath.Join(i.binDir(), name)
		os.Remove(link)
		if err := os.Symlink(target, link); err != nil {
			return err
		}
	}
	b, err := json.MarshalIndent(receipt{Name: p.pkg.Name, Version: p.pkg.Version, Languages: p.pkg.Languages, Bin: p.bins}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(dir, "receipt.json"), b)
}

func (i *Installer) Installed() ([]installer.Installed, error) {
	dirs, err := os.ReadDir(filepath.Join(i.opts.Dir, "packages"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []installer.Installed
	for _, d := range dirs {
		r, err := i.receipt(d.Name())
		if err != nil {
			continue // a partial or foreign directory
		}
		in := installer.Installed{Name: r.Name, Version: r.Version, Languages: r.Languages, Bin: map[string]string{}}
		for name := range r.Bin {
			in.Bin[name] = filepath.Join(i.binDir(), name)
		}
		out = append(out, in)
	}
	return out, nil
}

func (i *Installer) receipt(name string) (receipt, error) {
	var r receipt
	b, err := os.ReadFile(filepath.Join(i.packageDir(name), "receipt.json"))
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func (i *Installer) Remove(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("%w: %q", installer.ErrNotFound, name)
	}
	if r, err := i.receipt(name); err == nil {
		for bin := range r.Bin {
			if i.linkOwner(bin) == name {
				os.Remove(filepath.Join(i.binDir(), bin))
			}
		}
	}
	return os.RemoveAll(i.packageDir(name))
}
