package web_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

func TestPRPageShowsChecksAndConnectsForLiveUpdates(t *testing.T) {
	a := newApp(t)
	a.host.ChecksOf[ref] = domain.Checks{Head: c2, MergeState: domain.MergeBlocked, Checks: []domain.Check{
		{Name: "test", Workflow: "CI", State: domain.CheckFailure, Required: true, URL: "https://ci.example/run/1"},
		{Name: "lint", State: domain.CheckSuccess},
		{Name: "e2e", State: domain.CheckPending},
	}}

	body := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, body,
		`id="pr-status"`, `id="checks"`, "Blocked", "1 failing", `href="https://ci.example/run/1"`, "Required",
		`data-live="/github.com/o/r/pull/7/events?head=`+string(c2)+`"`, `id="refresh-banner"`, `id="offline-banner"`,
	)
}

func TestOfflinePageShowsTheBannerAndRefusesWrites(t *testing.T) {
	a := newApp(t)
	a.withThread()
	a.do(t, http.MethodGet, prPath, nil)
	a.host.SetOnline(false)

	body := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, body, "GitHub is unreachable", "data-offline")
	rec := a.do(t, http.MethodPost, prPath+"/threads/T1/replies", url.Values{"body": {"hi"}})
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("reply while offline: status %d, want 503", rec.Code)
	}
}

func TestRejectedCredentialsOpenTheLoginDialog(t *testing.T) {
	a := newApp(t)
	body := a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, body, `id="auth-failed"`)
	if strings.Contains(body, "data-open-on-load") {
		t.Error("login dialog opens while the credentials are accepted")
	}

	a.host.FailAddThread = fmt.Errorf("graphql: %w", codehost.ErrUnauthorized)
	a.host.Health.Unauthorized = true
	rec := a.do(t, http.MethodPost, prPath+"/threads", url.Values{
		"base": {string(mb)}, "head": {string(c2)}, "path": {"parser.go"}, "line": {"3"}, "side": {"RIGHT"}, "body": {"x"},
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("comment while rejected: status %d, want 401", rec.Code)
	}
	body = a.do(t, http.MethodGet, prPath, nil).Body.String()
	mustContain(t, body, `id="auth-failed"`, "data-open-on-load", "gh auth login")
}

func TestEventsStreamRendersUpdatesAsOutOfBandSwaps(t *testing.T) {
	a := newApp(t)
	srv := httptest.NewServer(a.handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+prPath+"/events", nil)
	req.Host = "127.0.0.1:7878"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	lines.Scan() // the connected comment, sent once subscribed
	lines.Scan() // and its blank line

	a.hub.Publish(ref, review.Update{
		Head:    "5555555555555555555555555555555555555555",
		Threads: []domain.ReviewThread{{ID: "T9", Path: "parser.go", Comments: []domain.Comment{{ID: "C9", BodyHTML: "<p>edited</p>"}}}},
		Pending: &domain.PendingReview{ID: "R", Comments: 3}, PendingChanged: true,
		PendingComments: []review.PendingComment{{ThreadID: "T9", Path: "parser.go", Line: 1, Body: "x"}},
	})

	var event []string
	for lines.Scan() && lines.Text() != "" {
		event = append(event, lines.Text())
	}
	got := strings.Join(event, "\n")
	mustContain(t, got,
		"event: update",
		`id="refresh-commits" hx-swap-oob="true"`, "New commits",
		`id="thread-T9"`, "<p>edited</p>",
		`id="pending-count" hx-swap-oob="true"`, "(3 pending)", `id="pending-list" hx-swap-oob="true"`,
	)
}

func TestEventsCatchUpAPageFromBeforeAPush(t *testing.T) {
	a := newApp(t)
	a.do(t, http.MethodGet, prPath, nil)
	srv := httptest.NewServer(a.handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+prPath+"/events?head="+string(c1), nil)
	req.Host = "127.0.0.1:7878"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	lines := bufio.NewScanner(resp.Body)
	var got []string
	for lines.Scan() && !strings.Contains(lines.Text(), "New commits") {
		got = append(got, lines.Text())
		if len(got) > 50 {
			t.Fatalf("no catch-up update: %q", got)
		}
	}
}

func TestStatusBoxSummarisesReviewsChecksAndMergeState(t *testing.T) {
	for _, c := range []struct {
		name      string
		checks    domain.Checks
		state     domain.PRState
		want      []string
		forbidden []string
	}{
		{"nothing enforced, nothing run", domain.Checks{MergeState: domain.MergeClean}, domain.Open,
			[]string{"No reviews yet", "No Checks"}, []string{"Ready to merge", `<details id="checks"`}},
		{"waiting on a required approval", domain.Checks{MergeState: domain.MergeBlocked, ReviewDecision: domain.ReviewRequired,
			Checks: []domain.Check{{Name: "ci", State: domain.CheckSuccess}, {Name: "lint", State: domain.CheckSuccess}}}, domain.Open,
			[]string{"Waiting on approval", "All 2 Checks passed", `<details id="checks"`, "Blocked"}, nil},
		{"changes requested", domain.Checks{MergeState: domain.MergeBlocked, ReviewDecision: domain.ChangesRequestedDecision,
			Verdicts: []domain.Verdict{{Reviewer: "ana", State: domain.VerdictApproved}, {Reviewer: "bo", State: domain.VerdictChangesRequested}},
			Checks:   []domain.Check{{Name: "ci", State: domain.CheckFailure}, {Name: "e2e", State: domain.CheckPending}, {Name: "lint", State: domain.CheckSuccess}}}, domain.Open,
			[]string{"Changes requested by bo", "1 failing", "1 pending", "1 passed"}, nil},
		{"approved and clean", domain.Checks{MergeState: domain.MergeClean, ReviewDecision: domain.ApprovedDecision,
			Verdicts: []domain.Verdict{{Reviewer: "ana", State: domain.VerdictApproved}},
			Checks:   []domain.Check{{Name: "ci", State: domain.CheckSuccess}}}, domain.Open,
			[]string{"Approved by ana", "Ready to merge"}, nil},
		{"draft", domain.Checks{MergeState: domain.MergeBlocked, Draft: true}, domain.Open, []string{"Draft"}, nil},
		{"merged", domain.Checks{MergeState: domain.MergeConflicting, ReviewDecision: domain.ApprovedDecision,
			Verdicts: []domain.Verdict{{Reviewer: "ana", State: domain.VerdictApproved}},
			Checks:   []domain.Check{{Name: "ci", State: domain.CheckSuccess}}}, domain.Merged,
			[]string{"Merged", "Approved by ana", "All 1 Check passed"}, []string{"Conflicts", "Waiting"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newApp(t)
			pr := a.host.PRs[ref]
			pr.State = c.state
			a.host.PRs[ref] = pr
			a.host.ChecksOf[ref] = c.checks
			body := a.do(t, http.MethodGet, prPath, nil).Body.String()
			i := strings.Index(body, `id="pr-status"`)
			if i < 0 {
				t.Fatal("no status box")
			}
			box := body[i:]
			box = box[:strings.Index(box, "</aside>")]
			mustContain(t, box, c.want...)
			for _, f := range c.forbidden {
				if strings.Contains(box, f) {
					t.Errorf("status box contains %q:\n%s", f, box)
				}
			}
		})
	}
}
