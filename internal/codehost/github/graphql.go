package github

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

//go:embed queries/review_state.graphql
var reviewStateQuery string

type gqlError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// graphql runs query with vars, decoding the response's data into out.
func (c *Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql: %s: %s", resp.Status, strings.TrimSpace(string(body[:min(len(body), 1024)])))
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if len(envelope.Errors) > 0 {
		e := envelope.Errors[0]
		if e.Type == "NOT_FOUND" {
			return fmt.Errorf("graphql: %s: %w", e.Message, codehost.ErrNotFound)
		}
		return fmt.Errorf("graphql: %s", e.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Data, out)
}

type gqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type gqlReactionGroup struct {
	Content          string `json:"content"`
	ViewerHasReacted bool   `json:"viewerHasReacted"`
	Reactors         struct {
		TotalCount int `json:"totalCount"`
	} `json:"reactors"`
}

type gqlComment struct {
	ID     string `json:"id"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Body            string             `json:"body"`
	BodyHTML        string             `json:"bodyHTML"`
	CreatedAt       time.Time          `json:"createdAt"`
	State           string             `json:"state"`
	ViewerDidAuthor bool               `json:"viewerDidAuthor"`
	ReactionGroups  []gqlReactionGroup `json:"reactionGroups"`
}

type gqlThread struct {
	ID                 string `json:"id"`
	IsResolved         bool   `json:"isResolved"`
	IsOutdated         bool   `json:"isOutdated"`
	Path               string `json:"path"`
	Line               *int   `json:"line"`
	StartLine          *int   `json:"startLine"`
	DiffSide           string `json:"diffSide"`
	StartDiffSide      string `json:"startDiffSide"`
	ViewerCanResolve   bool   `json:"viewerCanResolve"`
	ViewerCanUnresolve bool   `json:"viewerCanUnresolve"`
	Comments           struct {
		Nodes []gqlComment `json:"nodes"`
	} `json:"comments"`
}

type gqlReviewState struct {
	Viewer struct {
		Login string `json:"login"`
	} `json:"viewer"`
	Repository struct {
		PullRequest *struct {
			ID             string             `json:"id"`
			ReactionGroups []gqlReactionGroup `json:"reactionGroups"`
			Reviews        struct {
				Nodes []struct {
					ID     string `json:"id"`
					Commit *struct {
						OID string `json:"oid"`
					} `json:"commit"`
					Comments struct {
						TotalCount int `json:"totalCount"`
					} `json:"comments"`
				} `json:"nodes"`
			} `json:"reviews"`
			ReviewThreads struct {
				PageInfo gqlPageInfo `json:"pageInfo"`
				Nodes    []gqlThread `json:"nodes"`
			} `json:"reviewThreads"`
			Comments struct {
				Nodes []gqlComment `json:"nodes"`
			} `json:"comments"`
			Files struct {
				PageInfo gqlPageInfo `json:"pageInfo"`
				Nodes    []struct {
					Path              string `json:"path"`
					ViewerViewedState string `json:"viewerViewedState"`
				} `json:"nodes"`
			} `json:"files"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

func (c *Client) ReviewState(ctx context.Context, ref domain.PRRef) (domain.ReviewState, error) {
	state := domain.ReviewState{Viewed: map[string]domain.ViewedState{}}
	vars := map[string]any{"owner": ref.Owner, "name": ref.Name, "number": ref.Number, "threads": nil, "files": nil}
	for first := true; ; first = false {
		var page gqlReviewState
		if err := c.graphql(ctx, reviewStateQuery, vars, &page); err != nil {
			return domain.ReviewState{}, err
		}
		pr := page.Repository.PullRequest
		if pr == nil {
			return domain.ReviewState{}, fmt.Errorf("%s: %w", ref, codehost.ErrNotFound)
		}
		if first {
			state.PRID = pr.ID
			state.Viewer = page.Viewer.Login
			state.Reactions = reactions(pr.ReactionGroups)
			for _, c := range pr.Comments.Nodes {
				state.Conversation = append(state.Conversation, comment(c))
			}
			if n := pr.Reviews.Nodes; len(n) > 0 {
				p := &domain.PendingReview{ID: n[0].ID, Comments: n[0].Comments.TotalCount}
				if n[0].Commit != nil {
					p.Commit = domain.SHA(n[0].Commit.OID)
				}
				state.Pending = p
			}
		}
		for _, t := range pr.ReviewThreads.Nodes {
			state.Threads = append(state.Threads, thread(t))
		}
		for _, f := range pr.Files.Nodes {
			state.Viewed[f.Path] = domain.ViewedState(f.ViewerViewedState)
		}
		threadsMore, filesMore := pr.ReviewThreads.PageInfo.HasNextPage, pr.Files.PageInfo.HasNextPage
		if !threadsMore && !filesMore {
			return state, nil
		}
		// A finished connection is re-requested from its end, returning nothing.
		vars["threads"] = pr.ReviewThreads.PageInfo.EndCursor
		vars["files"] = pr.Files.PageInfo.EndCursor
	}
}

func thread(t gqlThread) domain.ReviewThread {
	th := domain.ReviewThread{
		ID:                 t.ID,
		Path:               t.Path,
		Side:               domain.DiffSide(t.DiffSide),
		StartSide:          domain.DiffSide(t.StartDiffSide),
		Resolved:           t.IsResolved,
		Outdated:           t.IsOutdated,
		ViewerCanResolve:   t.ViewerCanResolve,
		ViewerCanUnresolve: t.ViewerCanUnresolve,
	}
	if t.Line != nil && !t.IsOutdated {
		th.Line = *t.Line
	}
	if t.StartLine != nil && th.Line != 0 {
		th.StartLine = *t.StartLine
	}
	for _, c := range t.Comments.Nodes {
		th.Comments = append(th.Comments, comment(c))
	}
	return th
}

func comment(c gqlComment) domain.Comment {
	author := "ghost"
	if c.Author != nil {
		author = c.Author.Login
	}
	return domain.Comment{
		ID:              c.ID,
		Author:          author,
		Body:            c.Body,
		BodyHTML:        c.BodyHTML,
		CreatedAt:       c.CreatedAt,
		Pending:         c.State == "PENDING",
		ViewerDidAuthor: c.ViewerDidAuthor,
		Reactions:       reactions(c.ReactionGroups),
	}
}

func reactions(groups []gqlReactionGroup) []domain.ReactionGroup {
	out := make([]domain.ReactionGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, domain.ReactionGroup{
			Content:          domain.Reaction(g.Content),
			Count:            g.Reactors.TotalCount,
			ViewerHasReacted: g.ViewerHasReacted,
		})
	}
	return out
}
