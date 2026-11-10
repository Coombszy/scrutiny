package github

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

//go:embed queries/checks.graphql
var checksQuery string

type gqlChecks struct {
	Repository struct {
		PullRequest *struct {
			State                    string `json:"state"`
			IsDraft                  bool   `json:"isDraft"`
			ReviewDecision           string `json:"reviewDecision"`
			LatestOpinionatedReviews struct {
				Nodes []struct {
					State  string `json:"state"`
					Author *struct {
						Login string `json:"login"`
					} `json:"author"`
				} `json:"nodes"`
			} `json:"latestOpinionatedReviews"`
			MergeStateStatus  string `json:"mergeStateStatus"`
			HeadRefOid        string `json:"headRefOid"`
			StatusCheckRollup *struct {
				Contexts struct {
					Nodes []gqlCheckContext `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

// gqlCheckContext is a CheckRun or a StatusContext.
type gqlCheckContext struct {
	Typename   string `json:"__typename"`
	IsRequired bool   `json:"isRequired"`
	// CheckRun
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
	DetailsURL string  `json:"detailsUrl"`
	CheckSuite *struct {
		WorkflowRun *struct {
			Workflow struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
	// StatusContext
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

func (c *Client) Checks(ctx context.Context, ref domain.PRRef) (domain.Checks, error) {
	var out gqlChecks
	vars := map[string]any{"owner": ref.Owner, "name": ref.Name, "number": ref.Number}
	if err := c.graphql(ctx, checksQuery, vars, &out); err != nil {
		return domain.Checks{}, err
	}
	pr := out.Repository.PullRequest
	if pr == nil {
		return domain.Checks{}, fmt.Errorf("%s: %w", ref, codehost.ErrNotFound)
	}
	checks := domain.Checks{
		Head: domain.SHA(pr.HeadRefOid), MergeState: mergeState(pr.MergeStateStatus),
		PRState: domain.PRState(strings.ToLower(pr.State)), Draft: pr.IsDraft,
		ReviewDecision: domain.ReviewDecision(strings.ToLower(pr.ReviewDecision)),
	}
	for _, r := range pr.LatestOpinionatedReviews.Nodes {
		if r.Author == nil {
			continue
		}
		switch r.State {
		case "APPROVED":
			checks.Verdicts = append(checks.Verdicts, domain.Verdict{Reviewer: r.Author.Login, State: domain.VerdictApproved})
		case "CHANGES_REQUESTED":
			checks.Verdicts = append(checks.Verdicts, domain.Verdict{Reviewer: r.Author.Login, State: domain.VerdictChangesRequested})
		}
	}
	if pr.StatusCheckRollup != nil {
		for _, n := range pr.StatusCheckRollup.Contexts.Nodes {
			checks.Checks = append(checks.Checks, n.toDomain())
		}
	}
	return checks, nil
}

func (n gqlCheckContext) toDomain() domain.Check {
	if n.Typename == "StatusContext" {
		state := domain.CheckPending
		switch n.State {
		case "SUCCESS":
			state = domain.CheckSuccess
		case "FAILURE", "ERROR":
			state = domain.CheckFailure
		}
		return domain.Check{Name: n.Context, State: state, Required: n.IsRequired, URL: n.TargetURL}
	}
	check := domain.Check{Name: n.Name, State: domain.CheckPending, Required: n.IsRequired, URL: n.DetailsURL}
	if n.CheckSuite != nil && n.CheckSuite.WorkflowRun != nil {
		check.Workflow = n.CheckSuite.WorkflowRun.Workflow.Name
	}
	if n.Status == "COMPLETED" && n.Conclusion != nil {
		switch *n.Conclusion {
		case "SUCCESS":
			check.State = domain.CheckSuccess
		case "NEUTRAL", "SKIPPED", "STALE":
			check.State = domain.CheckNeutral
		default: // FAILURE, TIMED_OUT, CANCELLED, ACTION_REQUIRED, STARTUP_FAILURE
			check.State = domain.CheckFailure
		}
	}
	return check
}

func mergeState(s string) domain.MergeState {
	switch s {
	case "CLEAN", "HAS_HOOKS":
		return domain.MergeClean
	case "BLOCKED":
		return domain.MergeBlocked
	case "BEHIND":
		return domain.MergeBehind
	case "DIRTY":
		return domain.MergeConflicting
	case "UNSTABLE":
		return domain.MergeUnstable
	}
	return domain.MergeUnknown
}
