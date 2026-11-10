package github_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

func TestChecksMapsStatesRequiredAndMergeState(t *testing.T) {
	// Recorded from cli/go-gh#324, plus a commit status and two edge cases.
	var doc map[string]any
	json.Unmarshal(fixture(t, "checks.json"), &doc)
	rollup := doc["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["statusCheckRollup"].(map[string]any)
	ctxs := rollup["contexts"].(map[string]any)
	ctxs["nodes"] = append(ctxs["nodes"].([]any),
		map[string]any{"__typename": "StatusContext", "context": "ci/legacy", "state": "ERROR", "targetUrl": "https://ci.example/1", "isRequired": false},
		map[string]any{"__typename": "CheckRun", "name": "slow", "status": "IN_PROGRESS", "conclusion": nil, "detailsUrl": "https://x/2", "isRequired": true, "checkSuite": nil},
		map[string]any{"__typename": "CheckRun", "name": "docs", "status": "COMPLETED", "conclusion": "SKIPPED", "detailsUrl": "https://x/3", "isRequired": false, "checkSuite": nil},
	)
	body, _ := json.Marshal(doc)
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { return body })

	got, err := newClient(srv, cache.NewMemory()).Checks(context.Background(), pr325)
	if err != nil {
		t.Fatal(err)
	}
	if got.MergeState != domain.MergeBlocked || got.Head != "1a23985e6b8d6f8998d90c0e9eef3bfa556acd64" || len(got.Checks) != 14 {
		t.Fatalf("Checks = %s %s, %d checks", got.MergeState, got.Head, len(got.Checks))
	}
	first := domain.Check{Name: "codeql (go)", Workflow: "Code Scanning", State: domain.CheckSuccess, Required: true,
		URL: "https://github.com/cli/go-gh/actions/runs/42722668586/job/129520276410"}
	if got.Checks[0] != first {
		t.Errorf("Checks[0] = %+v", got.Checks[0])
	}
	tail := got.Checks[11:]
	if tail[0] != (domain.Check{Name: "ci/legacy", State: domain.CheckFailure, URL: "https://ci.example/1"}) ||
		tail[1].State != domain.CheckPending || !tail[1].Required || tail[2].State != domain.CheckNeutral {
		t.Errorf("tail = %+v", tail)
	}
}

func TestStatusTracksTheLowestRateLimit(t *testing.T) {
	body := fixture(t, "review_state.json")
	remaining := []string{"4000", "300"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", remaining[0])
		w.Header().Set("X-RateLimit-Reset", "1800000000")
		remaining = remaining[1:]
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	client := newClient(srv, cache.NewMemory())

	if st := client.Status(); st.RateLimit.Limit != 0 {
		t.Errorf("Status before any request = %+v", st)
	}
	client.ReviewState(context.Background(), pr14956)
	if st := client.Status(); !st.Online || st.RateLimit.Remaining != 4000 || st.RateLimit.Low() {
		t.Errorf("after first = %+v", st)
	}
	client.ReviewState(context.Background(), pr14956)
	if st := client.Status(); st.RateLimit.Remaining != 300 || !st.RateLimit.Low() || st.RateLimit.Reset.Unix() != 1800000000 {
		t.Errorf("after second = %+v", st)
	}
}

func TestUnreachableGitHubServesReadsFromCache(t *testing.T) {
	srv := fixtureServer(t, map[string]string{"/repos/cli/go-gh/pulls/325": "pull.json"})
	client := newClient(srv, cache.NewMemory())
	if _, err := client.PullRequest(context.Background(), pr325); err != nil {
		t.Fatal(err)
	}
	srv.Close()

	got, err := client.PullRequest(context.Background(), pr325)
	if err != nil || got.Title != "Fix auth token forwarding for hosts with ports" {
		t.Errorf("cached PullRequest = %q, %v", got.Title, err)
	}
	if client.Status().Online {
		t.Error("Status().Online = true after a failed request")
	}
	if _, err := client.Commits(context.Background(), pr325); !errors.Is(err, codehost.ErrOffline) {
		t.Errorf("uncached Commits: err = %v, want ErrOffline", err)
	}
	if _, err := client.ReviewState(context.Background(), pr14956); !errors.Is(err, codehost.ErrOffline) {
		t.Errorf("ReviewState: err = %v, want ErrOffline", err)
	}
}

func TestChecksIncludeReviewStatus(t *testing.T) {
	var doc map[string]any
	json.Unmarshal(fixture(t, "checks.json"), &doc) // cli/go-gh#324: a review is required, none given
	pr := doc["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { b, _ := json.Marshal(doc); return b })
	client := newClient(srv, cache.NewMemory())

	got, err := client.Checks(context.Background(), pr325)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReviewDecision != domain.ReviewRequired || len(got.Verdicts) != 0 || got.Draft || got.PRState != domain.Open {
		t.Errorf("recorded status = %+v", got)
	}

	pr["reviewDecision"] = "CHANGES_REQUESTED"
	pr["isDraft"] = true
	pr["latestOpinionatedReviews"] = map[string]any{"nodes": []any{
		map[string]any{"state": "APPROVED", "author": map[string]any{"login": "ana"}},
		map[string]any{"state": "CHANGES_REQUESTED", "author": map[string]any{"login": "bo"}},
	}}
	got, err = client.Checks(context.Background(), pr325)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Verdict{{Reviewer: "ana", State: domain.VerdictApproved}, {Reviewer: "bo", State: domain.VerdictChangesRequested}}
	if got.ReviewDecision != domain.ChangesRequestedDecision || !got.Draft || len(got.Verdicts) != 2 || got.Verdicts[0] != want[0] || got.Verdicts[1] != want[1] {
		t.Errorf("status = %+v", got)
	}
}
