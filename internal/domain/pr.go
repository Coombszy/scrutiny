package domain

import "time"

// PRState is whether a Pull Request is open, closed or merged.
type PRState string

const (
	Open   PRState = "open"
	Closed PRState = "closed"
	Merged PRState = "merged"
)

// PullRequest is a Pull Request's metadata.
type PullRequest struct {
	Ref          PRRef
	Title        string
	Body         string
	Author       string
	State        PRState
	Draft        bool
	BaseBranch   string
	HeadBranch   string
	HeadSHA      SHA
	URL          string // the PR on the code host
	Additions    int
	Deletions    int
	ChangedFiles int
	UpdatedAt    time.Time
}

// Commit is one commit on a PR's head branch.
type Commit struct {
	SHA        SHA
	Headline   string // first line of the message
	Author     string
	AuthoredAt time.Time
}

// PRSummary is a PR as listed on the Dashboard.
type PRSummary struct {
	Ref       PRRef
	Title     string
	Author    string
	Draft     bool
	UpdatedAt time.Time
}
