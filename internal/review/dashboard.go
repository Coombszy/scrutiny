package review

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
)

// recentLimit is how many recently opened PRs the Dashboard lists.
const recentLimit = 20

// dashboardMaxAge is how long the Dashboard's GitHub lists are shown before
// a visit fetches them again; the page also has a Refresh button.
const dashboardMaxAge = 10 * time.Minute

// DashboardPage is the PR lists shown on the Dashboard.
type DashboardPage struct {
	codehost.Dashboard
	Recent    []cache.Visit
	FetchedAt time.Time // when the GitHub lists were fetched
	Stale     bool      // older than dashboardMaxAge
}

const dashboardKey = "dashboard"

// Dashboard lists the PRs awaiting the viewer's review, those they authored
// and those recently opened in Scrutiny, keeping the lists for
// CachedDashboard.
func (s *Service) Dashboard(ctx context.Context) (DashboardPage, error) {
	d, err := s.CodeHost.Dashboard(ctx)
	if err != nil {
		return DashboardPage{}, err
	}
	b, err := json.Marshal(d)
	if err != nil {
		return DashboardPage{}, err
	}
	now := s.Now()
	if err := s.Cache.Put(ctx, dashboardKey, cache.Entry{Value: b, FetchedAt: now}); err != nil {
		return DashboardPage{}, err
	}
	recent, err := s.Cache.RecentVisits(ctx, recentLimit)
	return DashboardPage{Dashboard: d, Recent: recent, FetchedAt: now}, err
}

// CachedDashboard returns the Dashboard as last fetched, without calling
// the code host, so the page can render at once; ok is false when it has
// never been fetched. Recently opened PRs are always current.
func (s *Service) CachedDashboard(ctx context.Context) (page DashboardPage, ok bool, err error) {
	if page.Recent, err = s.Cache.RecentVisits(ctx, recentLimit); err != nil {
		return page, false, err
	}
	e, ok, err := s.Cache.Get(ctx, dashboardKey)
	if err != nil || !ok {
		return page, false, err
	}
	page.FetchedAt = e.FetchedAt
	page.Stale = s.Now().Sub(e.FetchedAt) > dashboardMaxAge
	return page, json.Unmarshal(e.Value, &page.Dashboard) == nil, nil
}
