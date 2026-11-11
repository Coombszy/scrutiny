package github_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
)

var pr14956 = domain.PRRef{Repo: domain.Repo{Host: "github.com", Owner: "cli", Name: "cli"}, Number: 14956}

type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// graphqlServer answers every POST /graphql with respond, recording requests.
func graphqlServer(t *testing.T, respond func(req gqlRequest) []byte) (*httptest.Server, *[]gqlRequest) {
	t.Helper()
	var reqs []gqlRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			http.NotFound(w, r)
			return
		}
		var req gqlRequest
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &req); err != nil {
			t.Fatalf("bad GraphQL request: %v", err)
		}
		reqs = append(reqs, req)
		w.Header().Set("Content-Type", "application/json")
		w.Write(respond(req))
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestReviewStateMapsThreadsFilesAndViewer(t *testing.T) {
	body := fixture(t, "review_state.json")
	srv, reqs := graphqlServer(t, func(gqlRequest) []byte { return body })

	got, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if err != nil {
		t.Fatal(err)
	}
	if v := (*reqs)[0].Variables; v["owner"] != "cli" || v["name"] != "cli" || v["number"] != float64(14956) {
		t.Errorf("variables = %v", v)
	}
	if got.PRID != "PR_kwDODKw3uc8AAAABNL00Hg" || got.Viewer != "Coombszy" || got.Pending != nil {
		t.Errorf("PRID %q, Viewer %q, Pending %+v", got.PRID, got.Viewer, got.Pending)
	}
	if len(got.Threads) != 4 || len(got.Viewed) != 5 {
		t.Fatalf("%d threads, %d files", len(got.Threads), len(got.Viewed))
	}
	th := got.Threads[0]
	if th.ID != "PRRT_kwDODKw3uc60TYx4" || th.Path != ".github/workflows/issue-triage.lock.yml" ||
		th.Line != 571 || th.StartLine != 567 || th.Side != domain.Right || !th.Resolved || th.Outdated {
		t.Errorf("thread = %+v", th)
	}
	c := th.Comments[0]
	if c.ID != "PRRC_kwDODKw3uc8AAAABC3Ffwg" || c.Author != "copilot-pull-request-reviewer" || c.Pending ||
		!c.CreatedAt.Equal(time.Date(2026, 11, 5, 22, 42, 20, 0, time.UTC)) || len(c.Reactions) != 8 {
		t.Errorf("comment = %+v", c)
	}
	if got.Viewed[".github/agents/agentic-workflows.md"] != domain.Unviewed {
		t.Errorf("Viewed = %v", got.Viewed)
	}
	for _, th := range got.Threads {
		if th.Outdated && th.Line != 0 {
			t.Errorf("outdated thread %s has line %d, want 0", th.ID, th.Line)
		}
	}
}

func TestReviewStateMapsConversationComments(t *testing.T) {
	body := fixture(t, "review_state_comments.json")
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { return body })

	got, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Conversation) != 12 || got.Conversation[0].ID != "IC_kwDODKw3uc8AAAABkeDe6w" || got.Conversation[0].Author != "cli-triage" {
		t.Errorf("Conversation = %d, first %+v", len(got.Conversation), got.Conversation[0])
	}
}

func TestReviewStateNotFound(t *testing.T) {
	body := fixture(t, "graphql_not_found.json")
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { return body })

	_, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if !errors.Is(err, codehost.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestReviewStateFollowsThreadPagination(t *testing.T) {
	var page1 map[string]any
	json.Unmarshal(fixture(t, "review_state.json"), &page1)
	threads := page1["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)["reviewThreads"].(map[string]any)
	threads["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "CURSOR1"}
	first, _ := json.Marshal(page1)
	srv, reqs := graphqlServer(t, func(req gqlRequest) []byte {
		if req.Variables["threads"] == "CURSOR1" {
			return fixture(t, "review_state.json")
		}
		return first
	})

	got, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if err != nil {
		t.Fatal(err)
	}
	if len(*reqs) != 2 || len(got.Threads) != 8 {
		t.Errorf("requests = %d, threads = %d; want 2 and 8", len(*reqs), len(got.Threads))
	}
}

func TestReviewStateMapsTheStack(t *testing.T) {
	body := fixture(t, "review_state.json") // cli/cli#14956 is 6th of 8 in a Stack
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { return body })

	got, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if err != nil {
		t.Fatal(err)
	}
	s := got.Stack
	if s == nil || s.Position != 6 || s.Size != 8 || len(s.Entries) != 8 {
		t.Fatalf("Stack = %+v", s)
	}
	if p := s.Parent(); p == nil || p.Number != 14955 || p.Title != "Add Git and configuration fixtures to ghmock" || p.State != domain.Merged {
		t.Errorf("Parent = %+v", p)
	}
	if c := s.Child(); c == nil || c.Number != 14957 {
		t.Errorf("Child = %+v", c)
	}
}

func TestReviewStateFindsTheViewersLastSubmittedReview(t *testing.T) {
	var doc map[string]any
	json.Unmarshal(fixture(t, "review_state.json"), &doc)
	pr := doc["data"].(map[string]any)["repository"].(map[string]any)["pullRequest"].(map[string]any)
	nodes := pr["submitted"].(map[string]any)["nodes"].([]any)
	nodes[1].(map[string]any)["viewerDidAuthor"] = true // 9e4e44a…
	nodes[3].(map[string]any)["viewerDidAuthor"] = true // 9e4e44a…, later
	body, _ := json.Marshal(doc)
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { return body })

	got, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got.LastReviewed), "9e4e44a") {
		t.Errorf("LastReviewed = %q", got.LastReviewed)
	}
}

func TestGraphQLErrorsAreLogged(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv, _ := graphqlServer(t, func(gqlRequest) []byte {
		return []byte(`{"data":null,"errors":[{"type":"FORBIDDEN","message":"Resource not accessible by integration"}]}`)
	})
	newClient(srv, cache.NewMemory()).SetViewed(context.Background(), "PR_1", "a.go", true)

	out := logs.String()
	for _, want := range []string{`msg="GitHub GraphQL error"`, "type=FORBIDDEN", `message="Resource not accessible by integration"`, "operation=markFileAsViewed"} {
		if !strings.Contains(out, want) {
			t.Errorf("logs lack %s:\n%s", want, out)
		}
	}
}

func TestReviewStateListsSubmittedReviews(t *testing.T) {
	body := fixture(t, "review_state_comments.json") // cli/cli#14998: reviewed by Copilot, approved by BagToad
	srv, _ := graphqlServer(t, func(gqlRequest) []byte { return body })

	got, err := newClient(srv, cache.NewMemory()).ReviewState(context.Background(), pr14956)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Reviews) != 2 {
		t.Fatalf("Reviews = %+v", got.Reviews)
	}
	r := got.Reviews[1]
	if r.Author != "BagToad" || r.Verdict != domain.ReviewApproved || !strings.HasPrefix(r.Body, "LGTM.") || r.BodyHTML == "" ||
		!r.SubmittedAt.Equal(time.Date(2026, 11, 9, 20, 35, 19, 0, time.UTC)) || r.ID == "" {
		t.Errorf("Reviews[1] = %+v", r)
	}
	if got.Reviews[0].Verdict != domain.ReviewCommented || got.Reviews[0].Author != "copilot-pull-request-reviewer" {
		t.Errorf("Reviews[0] = %+v", got.Reviews[0])
	}
}
