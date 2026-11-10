package github_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/codehost/github"
	"github.com/Coombszy/scrutiny/internal/domain"
)

// Mutations cannot be recorded without changing a real PR, so these check
// the operation and variables sent against hand-written responses shaped
// like GitHub's schema.
func TestMutationsSendTheRightOperation(t *testing.T) {
	const sha = domain.SHA("15fbe1db354aaa7ac0d966e5099f4c0064d4db02")
	cases := []struct {
		name     string
		call     func(*github.Client) error
		mutation string
		vars     map[string]any
	}{
		{
			"add thread",
			func(c *github.Client) error {
				return c.AddThread(context.Background(), "PRR_1", codehost.NewThread{Path: "a.go", Line: 12, StartLine: 10, Side: domain.Right, StartSide: domain.Right, Body: "nit"})
			},
			"addPullRequestReviewThread",
			map[string]any{"input": map[string]any{"pullRequestReviewId": "PRR_1", "path": "a.go", "line": float64(12), "side": "RIGHT", "startLine": float64(10), "startSide": "RIGHT", "body": "nit"}},
		},
		{
			"add single-line thread",
			func(c *github.Client) error {
				return c.AddThread(context.Background(), "PRR_1", codehost.NewThread{Path: "a.go", Line: 3, Side: domain.Left, Body: "why?"})
			},
			"addPullRequestReviewThread",
			map[string]any{"input": map[string]any{"pullRequestReviewId": "PRR_1", "path": "a.go", "line": float64(3), "side": "LEFT", "body": "why?"}},
		},
		{
			"reply",
			func(c *github.Client) error { return c.Reply(context.Background(), "PRR_1", "PRRT_1", "agreed") },
			"addPullRequestReviewThreadReply",
			map[string]any{"input": map[string]any{"pullRequestReviewId": "PRR_1", "pullRequestReviewThreadId": "PRRT_1", "body": "agreed"}},
		},
		{
			"resolve",
			func(c *github.Client) error { return c.SetThreadResolved(context.Background(), "PRRT_1", true) },
			"resolveReviewThread",
			map[string]any{"input": map[string]any{"threadId": "PRRT_1"}},
		},
		{
			"unresolve",
			func(c *github.Client) error { return c.SetThreadResolved(context.Background(), "PRRT_1", false) },
			"unresolveReviewThread",
			map[string]any{"input": map[string]any{"threadId": "PRRT_1"}},
		},
		{
			"conversation comment",
			func(c *github.Client) error { return c.AddConversationComment(context.Background(), "PR_1", "LGTM") },
			"addComment",
			map[string]any{"input": map[string]any{"subjectId": "PR_1", "body": "LGTM"}},
		},
		{
			"add reaction",
			func(c *github.Client) error { return c.SetReaction(context.Background(), "IC_1", domain.Rocket, true) },
			"addReaction",
			map[string]any{"input": map[string]any{"subjectId": "IC_1", "content": "ROCKET"}},
		},
		{
			"remove reaction",
			func(c *github.Client) error { return c.SetReaction(context.Background(), "IC_1", domain.Heart, false) },
			"removeReaction",
			map[string]any{"input": map[string]any{"subjectId": "IC_1", "content": "HEART"}},
		},
		{
			"submit pending review",
			func(c *github.Client) error {
				return c.SubmitReview(context.Background(), "PR_1", "PRR_1", sha, domain.Approve, "ship it")
			},
			"submitPullRequestReview",
			map[string]any{"input": map[string]any{"pullRequestReviewId": "PRR_1", "event": "APPROVE", "body": "ship it"}},
		},
		{
			"submit without pending review",
			func(c *github.Client) error {
				return c.SubmitReview(context.Background(), "PR_1", "", sha, domain.RequestChanges, "needs tests")
			},
			"addPullRequestReview",
			map[string]any{"input": map[string]any{"pullRequestId": "PR_1", "commitOID": string(sha), "event": "REQUEST_CHANGES", "body": "needs tests"}},
		},
		{
			"mark viewed",
			func(c *github.Client) error { return c.SetViewed(context.Background(), "PR_1", "a.go", true) },
			"markFileAsViewed",
			map[string]any{"input": map[string]any{"pullRequestId": "PR_1", "path": "a.go"}},
		},
		{
			"unmark viewed",
			func(c *github.Client) error { return c.SetViewed(context.Background(), "PR_1", "a.go", false) },
			"unmarkFileAsViewed",
			map[string]any{"input": map[string]any{"pullRequestId": "PR_1", "path": "a.go"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, reqs := graphqlServer(t, func(gqlRequest) []byte { return []byte(`{"data":{}}`) })
			if err := tc.call(newClient(srv, cache.NewMemory())); err != nil {
				t.Fatal(err)
			}
			req := (*reqs)[0]
			if !strings.Contains(req.Query, tc.mutation+"(input:") {
				t.Errorf("query %q does not call %s", req.Query, tc.mutation)
			}
			if !reflect.DeepEqual(req.Variables, tc.vars) {
				t.Errorf("variables = %v\nwant %v", req.Variables, tc.vars)
			}
		})
	}
}

func TestStartReviewReturnsTheNewReviewID(t *testing.T) {
	srv, reqs := graphqlServer(t, func(gqlRequest) []byte {
		return []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`)
	})

	id, err := newClient(srv, cache.NewMemory()).StartReview(context.Background(), "PR_1", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if id != "PRR_new" {
		t.Errorf("id = %q", id)
	}
	want := map[string]any{"input": map[string]any{"pullRequestId": "PR_1", "commitOID": "abc"}}
	if !reflect.DeepEqual((*reqs)[0].Variables, want) {
		t.Errorf("variables = %v", (*reqs)[0].Variables)
	}
}

func TestMutationErrorsAreReturned(t *testing.T) {
	srv, _ := graphqlServer(t, func(gqlRequest) []byte {
		return []byte(`{"data":null,"errors":[{"type":"FORBIDDEN","message":"Resource not accessible"}]}`)
	})
	err := newClient(srv, cache.NewMemory()).SetViewed(context.Background(), "PR_1", "a.go", true)
	if err == nil || !strings.Contains(err.Error(), "Resource not accessible") {
		t.Errorf("err = %v", err)
	}
}
