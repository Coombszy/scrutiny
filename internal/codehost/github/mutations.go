package github

import (
	"context"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

// mutation builds a single-input mutation document. Only the ID-like field
// is selected, since writers re-read ReviewState afterwards.
func mutation(name, inputType, selection string) string {
	return "mutation($input: " + inputType + "!) { " + name + "(input: $input) { " + selection + " } }"
}

var (
	addReview           = mutation("addPullRequestReview", "AddPullRequestReviewInput", "pullRequestReview { id }")
	addThread           = mutation("addPullRequestReviewThread", "AddPullRequestReviewThreadInput", "thread { id }")
	addThreadReply      = mutation("addPullRequestReviewThreadReply", "AddPullRequestReviewThreadReplyInput", "comment { id }")
	resolveThread       = mutation("resolveReviewThread", "ResolveReviewThreadInput", "thread { id }")
	unresolveThread     = mutation("unresolveReviewThread", "UnresolveReviewThreadInput", "thread { id }")
	addComment          = mutation("addComment", "AddCommentInput", "commentEdge { node { id } }")
	addReaction         = mutation("addReaction", "AddReactionInput", "reaction { content }")
	removeReaction      = mutation("removeReaction", "RemoveReactionInput", "reaction { content }")
	deleteReview        = mutation("deletePullRequestReview", "DeletePullRequestReviewInput", "pullRequestReview { id }")
	submitReview        = mutation("submitPullRequestReview", "SubmitPullRequestReviewInput", "pullRequestReview { id }")
	editReviewComment   = mutation("updatePullRequestReviewComment", "UpdatePullRequestReviewCommentInput", "pullRequestReviewComment { id }")
	deleteReviewComment = mutation("deletePullRequestReviewComment", "DeletePullRequestReviewCommentInput", "clientMutationId")
	editIssueComment    = mutation("updateIssueComment", "UpdateIssueCommentInput", "issueComment { id }")
	deleteIssueComment  = mutation("deleteIssueComment", "DeleteIssueCommentInput", "clientMutationId")
	markFileViewed      = mutation("markFileAsViewed", "MarkFileAsViewedInput", "clientMutationId")
	unmarkFileViewed    = mutation("unmarkFileAsViewed", "UnmarkFileAsViewedInput", "clientMutationId")
)

type input map[string]any

func (c *Client) mutate(ctx context.Context, doc string, in input, out any) error {
	return c.graphql(ctx, doc, map[string]any{"input": map[string]any(in)}, out)
}

func (c *Client) StartReview(ctx context.Context, prID string, commit domain.SHA) (string, error) {
	var out struct {
		AddPullRequestReview struct {
			PullRequestReview struct {
				ID string `json:"id"`
			} `json:"pullRequestReview"`
		} `json:"addPullRequestReview"`
	}
	err := c.mutate(ctx, addReview, input{"pullRequestId": prID, "commitOID": string(commit)}, &out)
	return out.AddPullRequestReview.PullRequestReview.ID, err
}

func (c *Client) AddThread(ctx context.Context, reviewID string, t codehost.NewThread) error {
	in := input{"pullRequestReviewId": reviewID, "path": t.Path, "line": t.Line, "side": string(t.Side), "body": t.Body}
	if t.StartLine != 0 && t.StartLine != t.Line {
		in["startLine"], in["startSide"] = t.StartLine, string(t.StartSide)
	}
	return c.mutate(ctx, addThread, in, nil)
}

// PostThread uses REST: GraphQL's addPullRequestReviewThread given only the
// PR starts a Pending Review rather than posting the thread.
func (c *Client) PostThread(ctx context.Context, ref domain.PRRef, commit domain.SHA, t codehost.NewThread) error {
	in := map[string]any{"commit_id": string(commit), "path": t.Path, "line": t.Line, "side": string(t.Side), "body": t.Body}
	if t.StartLine != 0 && t.StartLine != t.Line {
		in["start_line"], in["start_side"] = t.StartLine, string(t.StartSide)
	}
	return c.post(ctx, pullPath(ref)+"/comments", in)
}

func (c *Client) Reply(ctx context.Context, reviewID, threadID, body string) error {
	in := input{"pullRequestReviewThreadId": threadID, "body": body}
	if reviewID != "" {
		in["pullRequestReviewId"] = reviewID
	}
	return c.mutate(ctx, addThreadReply, in, nil)
}

func (c *Client) SetThreadResolved(ctx context.Context, threadID string, resolved bool) error {
	doc := unresolveThread
	if resolved {
		doc = resolveThread
	}
	return c.mutate(ctx, doc, input{"threadId": threadID}, nil)
}

func (c *Client) AddConversationComment(ctx context.Context, prID, body string) error {
	return c.mutate(ctx, addComment, input{"subjectId": prID, "body": body}, nil)
}

func (c *Client) SetReaction(ctx context.Context, subjectID string, r domain.Reaction, on bool) error {
	doc := removeReaction
	if on {
		doc = addReaction
	}
	return c.mutate(ctx, doc, input{"subjectId": subjectID, "content": string(r)}, nil)
}

func (c *Client) SubmitReview(ctx context.Context, prID, reviewID string, commit domain.SHA, event domain.ReviewEvent, body string) error {
	if reviewID == "" {
		return c.mutate(ctx, addReview, input{"pullRequestId": prID, "commitOID": string(commit), "event": string(event), "body": body}, nil)
	}
	return c.mutate(ctx, submitReview, input{"pullRequestReviewId": reviewID, "event": string(event), "body": body}, nil)
}

func (c *Client) DiscardReview(ctx context.Context, reviewID string) error {
	return c.mutate(ctx, deleteReview, input{"pullRequestReviewId": reviewID}, nil)
}

func (c *Client) SetViewed(ctx context.Context, prID, path string, viewed bool) error {
	doc := unmarkFileViewed
	if viewed {
		doc = markFileViewed
	}
	return c.mutate(ctx, doc, input{"pullRequestId": prID, "path": path}, nil)
}

func (c *Client) EditReviewComment(ctx context.Context, commentID, body string) error {
	return c.mutate(ctx, editReviewComment, input{"pullRequestReviewCommentId": commentID, "body": body}, nil)
}

func (c *Client) DeleteReviewComment(ctx context.Context, commentID string) error {
	return c.mutate(ctx, deleteReviewComment, input{"id": commentID}, nil)
}

func (c *Client) EditConversationComment(ctx context.Context, commentID, body string) error {
	return c.mutate(ctx, editIssueComment, input{"id": commentID, "body": body}, nil)
}

func (c *Client) DeleteConversationComment(ctx context.Context, commentID string) error {
	return c.mutate(ctx, deleteIssueComment, input{"id": commentID}, nil)
}
