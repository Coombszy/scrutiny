package review

import (
	"context"

	"github.com/Coombszy/scrutiny/internal/cache"
	"github.com/Coombszy/scrutiny/internal/codehost"
)

// recentLimit is how many recently opened PRs the Dashboard lists.
const recentLimit = 20

// DashboardPage is the PR lists shown on the Dashboard.
type DashboardPage struct {
	codehost.Dashboard
	Recent []cache.Visit
}

// Dashboard lists the PRs awaiting the viewer's review, those they authored
// and those recently opened in Scrutiny.
func (s *Service) Dashboard(ctx context.Context) (DashboardPage, error) {
	d, err := s.CodeHost.Dashboard(ctx)
	if err != nil {
		return DashboardPage{}, err
	}
	recent, err := s.Cache.RecentVisits(ctx, recentLimit)
	if err != nil {
		return DashboardPage{}, err
	}
	return DashboardPage{Dashboard: d, Recent: recent}, nil
}
