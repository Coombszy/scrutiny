package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			"single comment reply",
			func(c *github.Client) error { return c.Reply(context.Background(), "", "PRRT_1", "agreed") },
			"addPullRequestReviewThreadReply",
			map[string]any{"input": map[string]any{"pullRequestReviewThreadId": "PRRT_1", "body": "agreed"}},
		},
		{
			"discard review",
			func(c *github.Client) error { return c.DiscardReview(context.Background(), "PRR_1") },
			"deletePullRequestReview",
			map[string]any{"input": map[string]any{"pullRequestReviewId": "PRR_1"}},
		},
		{
			"edit review comment",
			func(c *github.Client) error { return c.EditReviewComment(context.Background(), "PRRC_1", "better") },
			"updatePullRequestReviewComment",
			map[string]any{"input": map[string]any{"pullRequestReviewCommentId": "PRRC_1", "body": "better"}},
		},
		{
			"delete review comment",
			func(c *github.Client) error { return c.DeleteReviewComment(context.Background(), "PRRC_1") },
			"deletePullRequestReviewComment",
			map[string]any{"input": map[string]any{"id": "PRRC_1"}},
		},
		{
			"edit conversation comment",
			func(c *github.Client) error { return c.EditConversationComment(context.Background(), "IC_1", "better") },
			"updateIssueComment",
			map[string]any{"input": map[string]any{"id": "IC_1", "body": "better"}},
		},
		{
			"delete conversation comment",
			func(c *github.Client) error { return c.DeleteConversationComment(context.Background(), "IC_1") },
			"deleteIssueComment",
			map[string]any{"input": map[string]any{"id": "IC_1"}},
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

func TestPostThreadCreatesAReviewCommentOverREST(t *testing.T) {
	type request struct {
		method, path string
		body         map[string]any
	}
	var got []request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("bad request body: %v", err)
		}
		got = append(got, request{r.Method, r.URL.Path, body})
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	c := newClient(srv, cache.NewMemory())
	ctx := context.Background()

	if err := c.PostThread(ctx, pr325, "abc", codehost.NewThread{Path: "a.go", Line: 12, StartLine: 10, Side: domain.Right, StartSide: domain.Right, Body: "nit"}); err != nil {
		t.Fatal(err)
	}
	if err := c.PostThread(ctx, pr325, "abc", codehost.NewThread{Path: "a.go", Line: 3, Side: domain.Left, Body: "why?"}); err != nil {
		t.Fatal(err)
	}
	want := []request{
		{http.MethodPost, "/repos/cli/go-gh/pulls/325/comments", map[string]any{
			"commit_id": "abc", "path": "a.go", "line": float64(12), "side": "RIGHT", "start_line": float64(10), "start_side": "RIGHT", "body": "nit"}},
		{http.MethodPost, "/repos/cli/go-gh/pulls/325/comments", map[string]any{
			"commit_id": "abc", "path": "a.go", "line": float64(3), "side": "LEFT", "body": "why?"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requests = %+v\nwant %+v", got, want)
	}
}

func TestPostThreadReturnsGitHubsRefusal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
	}))
	t.Cleanup(srv.Close)
	err := newClient(srv, cache.NewMemory()).PostThread(context.Background(), pr325, "abc", codehost.NewThread{Path: "a.go", Line: 1, Side: domain.Right, Body: "x"})
	if err == nil || !strings.Contains(err.Error(), "Validation Failed") {
		t.Errorf("err = %v", err)
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
