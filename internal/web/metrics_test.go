package web_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/metrics"
)

func TestSettingsShowActivityOnTheRight(t *testing.T) {
	a := newApp(t)
	a.metrics.Add(metrics.GitHubRequests, 12)
	a.metrics.Add(metrics.GitHubErrors, 2)
	a.host.Health = codehost.Status{Online: true, RateLimit: codehost.RateLimit{Remaining: 4812, Limit: 5000, Reset: time.Now().Add(20 * time.Minute), Resource: "search"}}

	page := a.do(t, http.MethodGet, "/settings", nil).Body.String()
	mustContain(t, page, `class="settings-layout"`, `id="activity-panel"`)
	panel := page[strings.Index(page, `id="activity-panel"`):]
	mustContain(t, panel,
		`hx-get="/settings/metrics"`, `hx-trigger="every 30s"`, "data-quiet",
		"GitHub requests", ">12<", "GitHub failures", ">2<",
		"GitHub search limit", "4,812 of 5,000", `class="spark"`,
	)

	fragment := a.do(t, http.MethodGet, "/settings/metrics", nil).Body.String()
	mustContain(t, fragment, `id="activity-panel"`, "GitHub requests")
	if strings.Contains(fragment, "<html") {
		t.Error("the refresh returns a whole page")
	}
}
