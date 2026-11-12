// Package release is the port for finding Scrutiny's latest release tag,
// and the Checker that looks for a New Version in the background.
package release

import (
	"context"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
)

// Source finds the latest release tag of a Go module.
type Source interface {
	// Latest returns the highest release version, e.g. "v0.3.0".
	Latest(ctx context.Context) (string, error)
}

// Module is Scrutiny's module path, as stamped into the binary.
const Module = "github.com/Coombszy/scrutiny"

// BuildVersion is the module version Go stamped into the binary: the tag for
// `go install …@v0.1.0` or a build of a clean tagged checkout, a
// pseudo-version for an untagged commit, "(devel)" for `go run`.
var BuildVersion = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
})

// pseudoVersion matches the timestamp and commit of a Go pseudo-version.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// IsTagged reports whether v is a tag rather than a dev build: not a
// pseudo-version, "(devel)" or a dirty (+) build.
func IsTagged(v string) bool {
	return strings.HasPrefix(v, "v") && !strings.Contains(v, "+") && !pseudoVersion.MatchString(v)
}

// URL is the GitHub page of tag v.
func URL(v string) string {
	return "https://" + Module + "/releases/tag/" + v
}
