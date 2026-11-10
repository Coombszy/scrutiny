package mason_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/installer/mason"
)

// command records one run of an external tool.
type command struct {
	Dir  string
	Env  []string
	Argv []string
}

type env struct {
	dir      string
	runs     []command
	onRun    func(c command)
	missing  map[string]bool
	assets   map[string][]byte // download path -> body
	mux      *http.ServeMux
	srv      *httptest.Server
	inst     *mason.Installer
	platform [2]string
}

// newEnv returns an Installer over testdata/registry.json, with external
// commands and downloads faked.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{dir: t.TempDir(), missing: map[string]bool{}, assets: map[string][]byte{}, platform: [2]string{"linux", "amd64"}}
	e.mux = http.NewServeMux()
	e.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, ok := e.assets[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(body)
	})
	e.srv = httptest.NewServer(e.mux)
	t.Cleanup(e.srv.Close)
	registry, err := os.ReadFile("testdata/registry.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.dir, "registry.json"), registry, 0o644); err != nil {
		t.Fatal(err)
	}
	e.inst = mason.New(mason.Options{
		Dir:          e.dir,
		HTTP:         e.srv.Client(),
		APIBase:      e.srv.URL + "/api",
		DownloadBase: e.srv.URL,
		GOOS:         e.platform[0],
		GOARCH:       e.platform[1],
		Run: func(_ context.Context, dir string, env []string, argv ...string) error {
			c := command{Dir: dir, Env: env, Argv: argv}
			e.runs = append(e.runs, c)
			if e.onRun != nil {
				e.onRun(c)
			}
			return nil
		},
		LookPath: func(name string) (string, error) {
			if e.missing[name] {
				return "", exec.ErrNotFound
			}
			return "/usr/bin/" + name, nil
		},
	})
	return e
}

func touch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func pkg(t *testing.T, ps []installer.Package, name string) installer.Package {
	t.Helper()
	i := slices.IndexFunc(ps, func(p installer.Package) bool { return p.Name == name })
	if i < 0 {
		t.Fatalf("package %s not listed", name)
	}
	return ps[i]
}

func TestPackagesListsLanguageServersAndWhatTheyNeed(t *testing.T) {
	e := newEnv(t)
	ps, err := e.inst.Packages(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(ps, func(p installer.Package) bool { return p.Name == "prettier" }) {
		t.Error("prettier is a formatter, not a Language Server")
	}
	for _, c := range []struct {
		name, source, version string
		requires              []string
		unsupported           bool
	}{
		{"gopls", "golang", "v0.23.0", []string{"go"}, false},
		{"typescript-language-server", "npm", "6.0.3", []string{"node", "npm"}, false},
		{"rust-analyzer", "github", "2026-11-09", nil, false},
		{"jdtls", "generic", "v1.63.0", nil, true},
		{"omnisharp", "github", "v1.39.15", nil, true}, // dotnet: bin
		{"solargraph", "gem", "0.62.1", nil, true},
	} {
		p := pkg(t, ps, c.name)
		if p.Source != c.source || p.Version != c.version || !slices.Equal(p.Requires, c.requires) || (p.Unsupported != "") != c.unsupported {
			t.Errorf("%s = source %q version %q requires %v unsupported %q", c.name, p.Source, p.Version, p.Requires, p.Unsupported)
		}
	}
	if langs := pkg(t, ps, "typescript-language-server").Languages; !slices.Equal(langs, []string{"TypeScript", "JavaScript"}) {
		t.Errorf("typescript-language-server languages = %v", langs)
	}
}

func TestMissingRuntimesAreReportedAndBlockInstall(t *testing.T) {
	e := newEnv(t)
	e.missing["node"] = true
	ps, _ := e.inst.Packages(context.Background(), false)

	if got := e.inst.Missing(pkg(t, ps, "pyright")); !slices.Equal(got, []string{"node"}) {
		t.Errorf("Missing(pyright) = %v, want [node]", got)
	}
	err := e.inst.Install(context.Background(), "pyright")
	if !errors.Is(err, installer.ErrMissingRuntime) || !strings.Contains(err.Error(), "node") {
		t.Errorf("Install(pyright) = %v, want ErrMissingRuntime naming node", err)
	}
	if len(e.runs) != 0 {
		t.Errorf("ran %v despite the missing runtime", e.runs)
	}
}

func TestInstallGoPackage(t *testing.T) {
	e := newEnv(t)
	e.onRun = func(c command) { touch(t, filepath.Join(c.Dir, "gopls")) }

	if err := e.inst.Install(context.Background(), "gopls"); err != nil {
		t.Fatal(err)
	}
	run := e.runs[0]
	pkgDir := filepath.Join(e.dir, "packages", "gopls")
	if !slices.Equal(run.Argv, []string{"go", "install", "golang.org/x/tools/gopls@v0.23.0"}) ||
		!slices.Contains(run.Env, "GOBIN="+run.Dir) || filepath.Dir(run.Dir) != filepath.Join(e.dir, "packages") {
		t.Errorf("ran %+v", run)
	}
	assertInstalled(t, e, "gopls", "v0.23.0", "gopls", filepath.Join(pkgDir, "gopls"))
}

func TestInstallNpmPackageWithExtraPackages(t *testing.T) {
	e := newEnv(t)
	e.onRun = func(c command) { touch(t, filepath.Join(c.Dir, "node_modules", ".bin", "typescript-language-server")) }

	if err := e.inst.Install(context.Background(), "typescript-language-server"); err != nil {
		t.Fatal(err)
	}
	pkgDir := filepath.Join(e.dir, "packages", "typescript-language-server")
	want := []string{"npm", "install", "--prefix", e.runs[0].Dir, "--no-save", "--no-audit", "--no-fund",
		"typescript-language-server@6.0.3", "typescript@6.0.3"}
	if !slices.Equal(e.runs[0].Argv, want) {
		t.Errorf("ran %v\nwant %v", e.runs[0].Argv, want)
	}
	assertInstalled(t, e, "typescript-language-server", "6.0.3", "typescript-language-server",
		filepath.Join(pkgDir, "node_modules", ".bin", "typescript-language-server"))
}

func TestInstallGitHubReleaseGzipForThisPlatform(t *testing.T) {
	e := newEnv(t)
	e.assets["/rust-lang/rust-analyzer/releases/download/2026-11-09/rust-analyzer-x86_64-unknown-linux-gnu.gz"] = gz(t, "ELF rust-analyzer")

	if err := e.inst.Install(context.Background(), "rust-analyzer"); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(e.dir, "packages", "rust-analyzer", "rust-analyzer-x86_64-unknown-linux-gnu")
	assertInstalled(t, e, "rust-analyzer", "2026-11-09", "rust-analyzer", bin)
	if b, _ := os.ReadFile(bin); string(b) != "ELF rust-analyzer" {
		t.Errorf("binary = %q", b)
	}
	if fi, _ := os.Stat(bin); fi.Mode()&0o111 == 0 {
		t.Errorf("binary mode %v is not executable", fi.Mode())
	}
}

func TestInstallGitHubReleaseTarballIntoADirectory(t *testing.T) {
	e := newEnv(t)
	e.platform = [2]string{"darwin", "arm64"}
	e = rebuild(t, e)
	e.assets["/LuaLS/lua-language-server/releases/download/3.19.2/lua-language-server-3.19.2-darwin-arm64.tar.gz"] =
		tarGz(t, map[string]string{"bin/lua-language-server": "lua!", "main.lua": "--"})

	if err := e.inst.Install(context.Background(), "lua-language-server"); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(e.dir, "packages", "lua-language-server", "libexec", "bin", "lua-language-server")
	assertInstalled(t, e, "lua-language-server", "3.19.2", "lua-language-server", bin)
}

func TestUnsupportedPackagesAreRefused(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"jdtls", "omnisharp"} {
		if err := e.inst.Install(context.Background(), name); !errors.Is(err, installer.ErrUnsupported) {
			t.Errorf("Install(%s) = %v, want ErrUnsupported", name, err)
		}
	}
	if err := e.inst.Install(context.Background(), "nope"); !errors.Is(err, installer.ErrNotFound) {
		t.Errorf("Install(nope) = %v, want ErrNotFound", err)
	}
}

func TestRemoveDeletesThePackageAndItsLinks(t *testing.T) {
	e := newEnv(t)
	e.onRun = func(c command) { touch(t, filepath.Join(c.Dir, "gopls")) }
	if err := e.inst.Install(context.Background(), "gopls"); err != nil {
		t.Fatal(err)
	}

	if err := e.inst.Remove("gopls"); err != nil {
		t.Fatal(err)
	}
	installed, _ := e.inst.Installed()
	if len(installed) != 0 {
		t.Errorf("Installed = %+v", installed)
	}
	for _, p := range []string{filepath.Join(e.dir, "packages", "gopls"), filepath.Join(e.dir, "bin", "gopls")} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s still exists", p)
		}
	}
}

func TestRegistryIsDownloadedAndVerified(t *testing.T) {
	e := newEnv(t)
	os.Remove(filepath.Join(e.dir, "registry.json"))
	registry, _ := os.ReadFile("testdata/registry.json")
	zipped := zipFile(t, "registry.json", registry)
	e.mux.HandleFunc("/api/repos/mason-org/mason-registry/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"2026-11-10-x","assets":[` +
			`{"name":"registry.json.zip","browser_download_url":"` + e.srv.URL + `/dl/registry.json.zip"},` +
			`{"name":"checksums.txt","browser_download_url":"` + e.srv.URL + `/dl/checksums.txt"}]}`))
	})
	e.assets["/dl/registry.json.zip"] = zipped
	e.assets["/dl/checksums.txt"] = []byte(sha256hex(zipped) + "  registry.json.zip\n")

	ps, err := e.inst.Packages(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 10 {
		t.Errorf("%d packages, want 10", len(ps))
	}

	e.assets["/dl/checksums.txt"] = []byte(strings.Repeat("0", 64) + "  registry.json.zip\n")
	if _, err := e.inst.Packages(context.Background(), true); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("tampered registry: err = %v, want a checksum error", err)
	}
	if ps, err := e.inst.Packages(context.Background(), false); err != nil || len(ps) != 10 {
		t.Errorf("cached registry after failed refresh: %d, %v", len(ps), err)
	}
}

func assertInstalled(t *testing.T, e *env, name, version, binName, target string) {
	t.Helper()
	installed, err := e.inst.Installed()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(installed, func(in installer.Installed) bool { return in.Name == name })
	if i < 0 {
		t.Fatalf("%s not installed: %+v", name, installed)
	}
	if installed[i].Version != version || installed[i].Bin[binName] != filepath.Join(e.dir, "bin", binName) {
		t.Errorf("Installed = %+v", installed[i])
	}
	link, err := filepath.EvalSymlinks(filepath.Join(e.dir, "bin", binName))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(target)
	if link != want {
		t.Errorf("bin/%s -> %s, want %s", binName, link, want)
	}
}

func rebuild(t *testing.T, old *env) *env {
	t.Helper()
	e := newEnv(t)
	e.platform = old.platform
	e.inst = mason.New(mason.Options{
		Dir: e.dir, HTTP: e.srv.Client(), APIBase: e.srv.URL + "/api", DownloadBase: e.srv.URL,
		GOOS: e.platform[0], GOARCH: e.platform[1],
		Run:      func(context.Context, string, []string, ...string) error { return nil },
		LookPath: func(n string) (string, error) { return "/usr/bin/" + n, nil },
	})
	return e
}

func gz(t *testing.T, content string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(content))
	w.Close()
	return b.Bytes()
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gw := gzip.NewWriter(&b)
	tw := tar.NewWriter(gw)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	tw.Close()
	gw.Close()
	return b.Bytes()
}

func tarWith(t *testing.T, entries []tar.Header, bodies map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gw := gzip.NewWriter(&b)
	tw := tar.NewWriter(gw)
	for _, h := range entries {
		h := h
		if h.Typeflag == tar.TypeReg {
			h.Size = int64(len(bodies[h.Name]))
		}
		tw.WriteHeader(&h)
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(bodies[h.Name]))
		}
	}
	tw.Close()
	gw.Close()
	return b.Bytes()
}

func TestArchivesCannotWriteOutsideThePackageThroughSymlinks(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "victim")
	os.WriteFile(outside, []byte("safe"), 0o644)
	const asset = "/LuaLS/lua-language-server/releases/download/3.19.2/lua-language-server-3.19.2-darwin-arm64.tar.gz"
	for name, entries := range map[string][]tar.Header{
		"absolute link": {
			{Name: "bin", Typeflag: tar.TypeSymlink, Linkname: outside},
			{Name: "bin", Typeflag: tar.TypeReg, Mode: 0o644},
		},
		"chained links": {
			{Name: "a", Typeflag: tar.TypeSymlink, Linkname: "."},
			{Name: "a/b", Typeflag: tar.TypeSymlink, Linkname: ".."},
			{Name: "a/b/evil", Typeflag: tar.TypeReg, Mode: 0o644},
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.platform = [2]string{"darwin", "arm64"}
			e = rebuild(t, e)
			e.assets[asset] = tarWith(t, entries, map[string]string{"bin": "pwned", "a/b/evil": "pwned"})
			if err := e.inst.Install(context.Background(), "lua-language-server"); err == nil {
				t.Error("Install succeeded")
			}
			if b, _ := os.ReadFile(outside); string(b) != "safe" {
				t.Error("wrote outside the package")
			}
			if _, err := os.Stat(filepath.Join(e.dir, "packages", "evil")); err == nil {
				t.Error("wrote beside the package")
			}
		})
	}
}

func TestAFailedUpdateKeepsTheInstalledVersion(t *testing.T) {
	e := newEnv(t)
	e.onRun = func(c command) { touch(t, filepath.Join(c.Dir, "gopls")) }
	if err := e.inst.Install(context.Background(), "gopls"); err != nil {
		t.Fatal(err)
	}
	e.onRun = nil // the next install produces no executable
	if err := e.inst.Install(context.Background(), "gopls"); err == nil {
		t.Fatal("broken update succeeded")
	}
	assertInstalled(t, e, "gopls", "v0.23.0", "gopls", filepath.Join(e.dir, "packages", "gopls", "gopls"))
}

func TestPackagesCannotTakeOverEachOthersExecutables(t *testing.T) {
	e := newEnv(t)
	e.onRun = func(c command) { touch(t, filepath.Join(c.Dir, "gopls")) }
	if err := e.inst.Install(context.Background(), "gopls"); err != nil {
		t.Fatal(err)
	}
	// A foreign link at a name another package wants is left alone.
	os.Symlink("/elsewhere/rust-analyzer", filepath.Join(e.dir, "bin", "rust-analyzer"))
	e.assets["/rust-lang/rust-analyzer/releases/download/2026-11-09/rust-analyzer-x86_64-unknown-linux-gnu.gz"] = gz(t, "ELF")
	if err := e.inst.Install(context.Background(), "rust-analyzer"); err == nil {
		t.Error("installed over an executable it does not own")
	}
	if err := e.inst.Remove("rust-analyzer"); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(filepath.Join(e.dir, "bin", "rust-analyzer")); target != "/elsewhere/rust-analyzer" {
		t.Errorf("Remove deleted a link it does not own (now %q)", target)
	}
}
