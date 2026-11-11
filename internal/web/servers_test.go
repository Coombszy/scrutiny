package web_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/navigator"
)

func TestServersPageShowsEachPackagesState(t *testing.T) {
	a := newApp(t)
	a.inst.Have["gopls"] = installer.Installed{Name: "gopls", Version: "v0.23.0", Languages: []string{"Go"}}
	a.inst.Absent["node"] = true

	body := a.do(t, http.MethodGet, "/settings", nil).Body.String()
	mustContain(t, body,
		"Language Servers", `id="server-gopls"`, "Installed v0.23.0", `hx-post="/settings/servers/gopls/remove"`,
		`id="server-pyright"`, "Requires node",
		`id="server-jdtls"`, "installs from generic",
		`hx-get="/settings/servers/list"`, `hx-post="/settings/servers/refresh"`,
	)
	if strings.Contains(body, `hx-post="/settings/servers/pyright/install"`) {
		t.Error("pyright offers Install although node is missing")
	}
}

func TestInstallingShowsProgressThenTheInstalledVersion(t *testing.T) {
	a := newApp(t)
	a.inst.Block = make(chan struct{})

	rec := a.do(t, http.MethodPost, "/settings/servers/gopls/install", formValues())
	mustContain(t, rec.Body.String(), `id="server-gopls"`, "Installing", `hx-trigger="every 1s"`, "data-quiet")

	close(a.inst.Block)
	deadline := time.Now().Add(5 * time.Second)
	for {
		body := a.do(t, http.MethodGet, "/settings/servers/gopls", nil).Body.String()
		if strings.Contains(body, "Installed v0.23.0") {
			if strings.Contains(body, "every 1s") {
				t.Error("still polling once installed")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never installed: %s", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRemovingAServer(t *testing.T) {
	a := newApp(t)
	a.inst.Have["gopls"] = installer.Installed{Name: "gopls", Version: "v0.23.0"}
	body := a.do(t, http.MethodPost, "/settings/servers/gopls/remove", formValues()).Body.String()
	mustContain(t, body, `hx-post="/settings/servers/gopls/install"`)
}

func TestServerSearchFilters(t *testing.T) {
	a := newApp(t)
	body := a.do(t, http.MethodGet, "/settings/servers/list?q=pyth", nil).Body.String()
	mustContain(t, body, "pyright")
	if strings.Contains(body, "gopls") {
		t.Error("gopls matched pyth")
	}
	if rec := a.do(t, http.MethodGet, "/settings/servers/nope", nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown server: status %d", rec.Code)
	}
}

func TestAnInstalledServersLogFoldsUnderItsRow(t *testing.T) {
	a := newApp(t)
	a.inst.Have["gopls"] = installer.Installed{Name: "gopls", Version: "v0.23.0", Languages: []string{"Go"},
		Bin: map[string]string{"gopls": "/servers/bin/gopls"}}

	row := a.do(t, http.MethodGet, "/settings/servers/gopls", nil).Body.String()
	mustContain(t, row, `<details class="server-log">`, `hx-get="/settings/servers/gopls/log"`, "toggle from:closest details once")
	mustContain(t, a.do(t, http.MethodGet, "/settings/servers/gopls/log", nil).Body.String(), "No output yet")

	at := time.Date(2026, 11, 11, 9, 30, 0, 0, time.UTC)
	a.nav.Logs["/servers/bin/gopls"] = []navigator.LogEntry{
		{Time: at, Level: navigator.LogInfo, Source: "scrutiny", Text: "starting gopls"},
		{Time: at.Add(time.Second), Level: navigator.LogError, Source: "server", Text: "go.mod <not found>"},
	}
	row = a.do(t, http.MethodGet, "/settings/servers/gopls", nil).Body.String()
	mustContain(t, row, "<details class=\"server-log\" open", "· 1 error", `hx-trigger="load"`)
	log := a.do(t, http.MethodGet, "/settings/servers/gopls/log", nil).Body.String()
	mustContain(t, log, `class="server-log-lines"`, "09:30:01", `class="log-error"`, "go.mod &lt;not found&gt;", "starting gopls")
	if strings.Index(log, "starting gopls") > strings.Index(log, "go.mod") {
		t.Error("the log is not oldest first")
	}

	if body := a.do(t, http.MethodGet, "/settings/servers/pyright", nil).Body.String(); strings.Contains(body, "server-log") {
		t.Error("a server that is not installed has a log")
	}
}
