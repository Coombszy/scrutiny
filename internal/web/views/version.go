package views

import (
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
)

// buildVersion is the module version Go stamped into the binary: the tag for
// `go install …@v0.1.0` or a build of a clean tagged checkout, a
// pseudo-version for an untagged commit, "(devel)" for `go run`.
var buildVersion = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
})

// pseudoVersion matches the timestamp and commit of a Go pseudo-version.
var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// versionLabel is what the top bar shows for version v: a release tag as it
// is, anything else as "dev".
func versionLabel(v string) string {
	if !strings.HasPrefix(v, "v") || strings.Contains(v, "+") || pseudoVersion.MatchString(v) {
		return "dev"
	}
	return v
}
