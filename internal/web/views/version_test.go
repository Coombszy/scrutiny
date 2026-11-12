package views

import "testing"

func TestVersionLabel(t *testing.T) {
	for v, want := range map[string]string{
		"v0.1.0":                               "v0.1.0",
		"v1.2.0-rc.1":                          "v1.2.0-rc.1",
		"v0.1.1-0.20261112093000-7d7b0b3abcde": "dev",
		"v0.0.0-20261112093000-7d7b0b3abcde":   "dev",
		"v0.1.0+dirty":                         "dev",
		"(devel)":                              "dev",
		"":                                     "dev",
	} {
		if got := versionLabel(v); got != want {
			t.Errorf("versionLabel(%q) = %q, want %q", v, got, want)
		}
	}
}
