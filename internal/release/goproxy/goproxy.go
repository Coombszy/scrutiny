// Package goproxy finds a module's latest release from a Go module proxy
// (https://go.dev/ref/mod#goproxy-protocol), as `go install …@latest` would.
package goproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/mod/module"
)

// DefaultProxy is the proxy used when GOPROXY is unset.
const DefaultProxy = "https://proxy.golang.org"

// ErrNoProxy means GOPROXY names no proxy to ask ("off" or "direct" only).
var ErrNoProxy = errors.New("goproxy: GOPROXY names no proxy")

// Source asks a proxy for Module's latest release.
type Source struct {
	Proxy  string // base URL, e.g. DefaultProxy; empty asks nothing
	Module string
	Client *http.Client
}

// New returns a Source for module through the first proxy in goproxy, a
// GOPROXY value ("" means the default).
func New(goproxy, module string) *Source {
	return &Source{Proxy: FirstProxy(goproxy), Module: module, Client: &http.Client{Timeout: 30 * time.Second}}
}

// FirstProxy is the first proxy URL in a GOPROXY list, before any "off" or
// "direct"; "" if there is none.
func FirstProxy(goproxy string) string {
	if goproxy == "" {
		return DefaultProxy
	}
	for _, p := range strings.FieldsFunc(goproxy, func(r rune) bool { return r == ',' || r == '|' }) {
		p = strings.TrimSpace(p)
		if p == "off" || p == "direct" {
			return ""
		}
		if strings.HasPrefix(p, "https://") || strings.HasPrefix(p, "http://") {
			return strings.TrimSuffix(p, "/")
		}
	}
	return ""
}

// Latest returns the version the proxy's $module/@latest names.
func (s *Source) Latest(ctx context.Context) (string, error) {
	if s.Proxy == "" {
		return "", ErrNoProxy
	}
	path, err := module.EscapePath(s.Module)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Proxy+"/"+path+"/@latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("goproxy: %s: %s", req.URL, resp.Status)
	}
	var info struct{ Version string }
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("goproxy: %s: %w", req.URL, err)
	}
	return info.Version, nil
}
