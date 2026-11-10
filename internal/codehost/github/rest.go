package github

import (
	"strings"
	"time"

	"github.com/Coombszy/scrutiny/internal/domain"
)

type restUser struct {
	Login string `json:"login"`
}

type restPull struct {
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	State        string     `json:"state"`
	Draft        bool       `json:"draft"`
	MergedAt     *time.Time `json:"merged_at"`
	HTMLURL      string     `json:"html_url"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	ChangedFiles int        `json:"changed_files"`
	UpdatedAt    time.Time  `json:"updated_at"`
	User         restUser   `json:"user"`
	Base         struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
}

func (p restPull) toDomain(ref domain.PRRef) domain.PullRequest {
	state := domain.PRState(p.State)
	if p.MergedAt != nil {
		state = domain.Merged
	}
	return domain.PullRequest{
		Ref:          ref,
		Title:        p.Title,
		Body:         p.Body,
		Author:       p.User.Login,
		State:        state,
		Draft:        p.Draft,
		BaseBranch:   p.Base.Ref,
		HeadBranch:   p.Head.Ref,
		BaseSHA:      domain.SHA(p.Base.SHA),
		HeadSHA:      domain.SHA(p.Head.SHA),
		URL:          p.HTMLURL,
		Additions:    p.Additions,
		Deletions:    p.Deletions,
		ChangedFiles: p.ChangedFiles,
		UpdatedAt:    p.UpdatedAt,
	}
}

type restCommit struct {
	SHA    string    `json:"sha"`
	Author *restUser `json:"author"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

func (c restCommit) toDomain() domain.Commit {
	author := c.Commit.Author.Name
	if c.Author != nil && c.Author.Login != "" {
		author = c.Author.Login
	}
	headline, _, _ := strings.Cut(c.Commit.Message, "\n")
	return domain.Commit{
		SHA:        domain.SHA(c.SHA),
		Headline:   headline,
		Author:     author,
		AuthoredAt: c.Commit.Author.Date,
	}
}

type restSearch struct {
	Items []restIssue `json:"items"`
}

type restIssue struct {
	Number        int       `json:"number"`
	Title         string    `json:"title"`
	Draft         bool      `json:"draft"`
	UpdatedAt     time.Time `json:"updated_at"`
	User          restUser  `json:"user"`
	RepositoryURL string    `json:"repository_url"` // https://api.github.com/repos/<owner>/<repo>
}

func (i restIssue) toDomain() (domain.PRSummary, bool) {
	_, ownerRepo, ok := strings.Cut(i.RepositoryURL, "/repos/")
	owner, repo, ok2 := strings.Cut(ownerRepo, "/")
	if !ok || !ok2 {
		return domain.PRSummary{}, false
	}
	return domain.PRSummary{
		Ref:       domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: owner, Name: repo}, Number: i.Number},
		Title:     i.Title,
		Author:    i.User.Login,
		Draft:     i.Draft,
		UpdatedAt: i.UpdatedAt,
	}, true
}
