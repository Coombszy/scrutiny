// Package domain holds the core types shared by every layer.
package domain

import (
	"fmt"
	"strconv"
)

// Repo identifies a repository on a code host.
type Repo struct {
	Host  string // e.g. github.com
	Owner string
	Name  string
}

func (r Repo) String() string { return r.Host + "/" + r.Owner + "/" + r.Name }

// PRRef identifies a Pull Request.
type PRRef struct {
	Repo
	Number int
}

func (p PRRef) String() string { return fmt.Sprintf("%s#%d", p.Repo, p.Number) }

// Path is the PR's URL path in Scrutiny, mirroring GitHub's with the host
// included, e.g. /github.com/owner/repo/pull/123.
func (p PRRef) Path() string {
	return "/" + p.Repo.String() + "/pull/" + strconv.Itoa(p.Number)
}

// SHA is a full commit object ID.
type SHA string

// Short returns the abbreviated SHA for display.
func (s SHA) Short() string {
	if len(s) > 7 {
		return string(s[:7])
	}
	return string(s)
}
