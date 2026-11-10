package mason

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// spec is a package as the Mason registry describes it.
type spec struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Homepage    string            `json:"homepage"`
	Languages   []string          `json:"languages"`
	Categories  []string          `json:"categories"`
	Source      source            `json:"source"`
	Bin         map[string]string `json:"bin"`
}

type source struct {
	ID            string          `json:"id"`
	ExtraPackages []string        `json:"extra_packages"`
	Asset         json.RawMessage `json:"asset"` // one asset or a list
	Build         json.RawMessage `json:"build"`
}

type asset struct {
	Target json.RawMessage `json:"target"` // a string or a list
	File   string          `json:"file"`
	Bin    string          `json:"bin"`
}

func (a asset) targets() []string {
	var many []string
	if json.Unmarshal(a.Target, &many) == nil {
		return many
	}
	var one string
	_ = json.Unmarshal(a.Target, &one)
	return []string{one}
}

// purl is a parsed package URL, e.g. pkg:npm/%40scope/name@1.0.0#sub.
type purl struct {
	Type      string
	Namespace string // e.g. "golang.org/x/tools" or the GitHub owner
	Name      string
	Version   string
	Subpath   string
}

func parsePURL(id string) (purl, error) {
	rest, ok := strings.CutPrefix(id, "pkg:")
	if !ok {
		return purl{}, fmt.Errorf("not a package URL: %q", id)
	}
	rest, sub, _ := strings.Cut(rest, "#")
	rest, _, _ = strings.Cut(rest, "?")
	typ, rest, ok := strings.Cut(rest, "/")
	if !ok {
		return purl{}, fmt.Errorf("package URL without a name: %q", id)
	}
	rest, version, _ := strings.Cut(rest, "@")
	rest, err := url.PathUnescape(rest)
	if err != nil {
		return purl{}, err
	}
	p := purl{Type: typ, Name: rest, Version: version, Subpath: sub}
	if i := strings.LastIndex(rest, "/"); i >= 0 {
		p.Namespace, p.Name = rest[:i], rest[i+1:]
	}
	return p, nil
}

// fullName is the namespace and name, e.g. "@scope/name" or a Go module path.
func (p purl) fullName() string {
	if p.Namespace == "" {
		return p.Name
	}
	return p.Namespace + "/" + p.Name
}

const registryFile = "registry.json"

// loadRegistry reads the cached registry, downloading it first when asked
// to or when there is none.
func (i *Installer) loadRegistry(ctx context.Context, refresh bool) ([]spec, error) {
	path := filepath.Join(i.opts.Dir, registryFile)
	if _, err := os.Stat(path); refresh || errors.Is(err, os.ErrNotExist) {
		if err := i.downloadRegistry(ctx, path); err != nil {
			return nil, fmt.Errorf("update Language Server registry: %w", err)
		}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var specs []spec
	return specs, json.Unmarshal(b, &specs)
}

// downloadRegistry fetches the latest mason-registry release's
// registry.json.zip, checks it against the release's checksums.txt and
// stores the registry at path.
func (i *Installer) downloadRegistry(ctx context.Context, path string) error {
	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	body, err := i.get(ctx, i.opts.APIBase+"/repos/mason-org/mason-registry/releases/latest")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return err
	}
	urls := map[string]string{}
	for _, a := range release.Assets {
		urls[a.Name] = a.URL
	}
	if urls["registry.json.zip"] == "" || urls["checksums.txt"] == "" {
		return errors.New("release is missing registry.json.zip or checksums.txt")
	}
	zipped, err := i.get(ctx, urls["registry.json.zip"])
	if err != nil {
		return err
	}
	sums, err := i.get(ctx, urls["checksums.txt"])
	if err != nil {
		return err
	}
	sum := sha256.Sum256(zipped)
	if want := checksumFor(string(sums), "registry.json.zip"); want == "" || want != hex.EncodeToString(sum[:]) {
		return errors.New("registry.json.zip does not match its checksum")
	}
	zr, err := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if err != nil {
		return err
	}
	f, err := zr.Open(registryFile)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}

func checksumFor(sums, name string) string {
	for _, line := range strings.Split(sums, "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == name {
			return fields[0]
		}
	}
	return ""
}

func (i *Installer) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := i.opts.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
