package web

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
)

type prJSON struct {
	Title  string `json:"title"`
	Ref    string `json:"ref"`
	Author string `json:"author,omitempty"`
	Draft  bool   `json:"draft,omitempty"`
	Href   string `json:"href"`
}

// prsJSON is the PR search's list; Stale says the Dashboard's lists were
// never fetched or are over ten minutes old, so the search asks again with
// refresh set.
type prsJSON struct {
	PRs   []prJSON `json:"prs"`
	Stale bool     `json:"stale"`
}

// prs lists the PRs the PR search offers on every page: the Dashboard's
// lists, then those recently opened, once each. It answers from the cache
// at once; with refresh set it fetches the lists from GitHub first, as the
// Dashboard does, finishing the fetch even if the browser leaves.
func (s *Server) prs(w http.ResponseWriter, r *http.Request) {
	var (
		d      review.DashboardPage
		cached bool
		err    error
	)
	if r.URL.Query().Get("refresh") != "" {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), dashboardFetchTimeout)
		defer cancel()
		d, err = s.review.Dashboard(ctx)
		cached = true
	} else {
		d, cached, err = s.review.CachedDashboard(r.Context())
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := prsJSON{PRs: []prJSON{}, Stale: !cached || d.Stale}
	seen := map[domain.PRRef]bool{}
	add := func(p prJSON, ref domain.PRRef) {
		if !seen[ref] {
			seen[ref] = true
			p.Ref, p.Href = ref.String(), ref.Path()
			out.PRs = append(out.PRs, p)
		}
	}
	for _, list := range [][]domain.PRSummary{d.ReviewRequested, d.Authored} {
		for _, p := range list {
			add(prJSON{Title: p.Title, Author: p.Author, Draft: p.Draft}, p.Ref)
		}
	}
	for _, v := range d.Recent {
		add(prJSON{Title: v.Title}, v.PR)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
