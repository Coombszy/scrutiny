package web_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

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
		`id="checks"`, "Blocked", `class="check-count failure"`, `href="https://ci.example/run/1"`, "Required",
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
		`id="pending-count" hx-swap-oob="true"`, ">3<",
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

func TestMergeStateIsShownOnlyWhereItMeansSomething(t *testing.T) {
	for _, c := range []struct {
		name      string
		checks    domain.Checks
		state     domain.PRState
		want      string
		forbidden string
	}{
		{"clean with no checks", domain.Checks{MergeState: domain.MergeClean}, domain.Open, "No Checks", "Ready to merge"},
		{"clean with checks", domain.Checks{MergeState: domain.MergeClean, Checks: []domain.Check{{Name: "ci", State: domain.CheckSuccess}}}, domain.Open, "Ready to merge", ""},
		{"blocked with no checks", domain.Checks{MergeState: domain.MergeBlocked}, domain.Open, "Blocked", ""},
		{"merged", domain.Checks{MergeState: domain.MergeConflicting, Checks: []domain.Check{{Name: "ci", State: domain.CheckSuccess}}}, domain.Merged, "✓ 1", "Conflicts"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newApp(t)
			pr := a.host.PRs[ref]
			pr.State = c.state
			a.host.PRs[ref] = pr
			a.host.ChecksOf[ref] = c.checks
			body := a.do(t, http.MethodGet, prPath, nil).Body.String()
			checks := body[strings.Index(body, `id="checks"`):]
			checks = checks[:strings.Index(checks, "</summary>")]
			mustContain(t, checks, c.want)
			if c.forbidden != "" && strings.Contains(checks, c.forbidden) {
				t.Errorf("Checks pill shows %q: %s", c.forbidden, checks)
			}
		})
	}
}
