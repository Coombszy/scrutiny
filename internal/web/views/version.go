package views

import "github.com/Coombszy/scrutiny/internal/release"

// buildVersion is the running build's module version.
var buildVersion = release.BuildVersion

// versionLabel is what the top bar shows for version v: a tag as it is,
// anything else as "dev".
func versionLabel(v string) string {
	if !release.IsTagged(v) {
		return "dev"
	}
	return v
}

// newVersionTitle is the tooltip of the New Version link.
func newVersionTitle(v string) string {
	return "Scrutiny " + v + " is available (running " + buildVersion() + "): go install " + release.Module + "/cmd/scrutiny@" + v
}
