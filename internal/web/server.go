// Package web is the driving HTTP adapter: it serves the Templ + htmx UI.
package web

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/live"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/repostore"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/servers"
	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/theme"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

// Deps are what the Server serves from.
type Deps struct {
	// CSRFToken must accompany every mutating request.
	CSRFToken string
	Review    *review.Service
	Settings  settings.Store
	Hub       *live.Hub // live updates for open pages
	Servers   *servers.Service
	ThemesDir string // drop-in Themes; may be empty
	// Dev relaxes the Content-Security-Policy for a live-reload proxy.
	Dev bool
}

// Server serves the UI.
type Server struct {
	csrfToken string
	review    *review.Service
	settings  settings.Store
	hub       *live.Hub
	servers   *servers.Service
	themesDir string
	dev       bool
	mux       *http.ServeMux
}

// NewServer returns a Server over d.
func NewServer(d Deps) *Server {
	s := &Server{
		csrfToken: d.CSRFToken, review: d.Review, settings: d.Settings, hub: d.Hub, servers: d.Servers,
		themesDir: d.ThemesDir, dev: d.Dev, mux: http.NewServeMux(),
	}
	static, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("GET /{$}", s.dashboard)
	s.mux.HandleFunc("GET /open", s.open)
	s.mux.HandleFunc("GET /dashboard/lists", s.dashboardLists)
	s.mux.HandleFunc("GET /github.com/{owner}/{repo}/pull/{number}", s.pullRequest)
	s.mux.HandleFunc("POST /github.com/{owner}/{repo}/pull/{number}/forget", s.forgetPR)
	s.mux.HandleFunc("GET /github.com/{owner}/{repo}/pull/{number}/file", s.file)
	s.mux.HandleFunc("GET /github.com/{owner}/{repo}/pull/{number}/context", s.expandGap)
	s.mux.HandleFunc("GET /settings", s.settingsPage)
	s.mux.HandleFunc("POST /settings", s.saveSettings)
	s.mux.HandleFunc("POST /settings/view-mode", s.setViewMode)
	s.mux.HandleFunc("POST /settings/theme", s.setTheme)
	s.mux.HandleFunc("POST /settings/keys", s.setKey)
	s.mux.HandleFunc("GET /theme.css", s.themeCSS)
	s.mux.HandleFunc("GET /github.com/{owner}/{repo}/pull/{number}/events", s.events)
	s.routeNavigation()
	s.routeServers()
	s.routeReview()
	return s
}

// Handler returns the root handler with the security middleware applied.
func (s *Server) Handler() http.Handler {
	if s.dev {
		return secure(s.mux, s.csrfToken, devContentSecurityPolicy)
	}
	return Secure(s.mux, s.csrfToken)
}

type component interface {
	Render(ctx context.Context, w io.Writer) error
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, cs ...component) {
	s.renderPage(w, r, status, views.Page{}, cs...)
}

// renderPage renders with page's per-page data; the CSRF token, Theme,
// hotkeys and View Mode are filled in.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, status int, page views.Page, cs ...component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	page.CSRFToken = s.csrfToken
	if cfg, err := s.settings.Load(); err == nil {
		page.Theme, page.Keys, page.ViewMode = cfg.Theme, cfg.Keys, string(cfg.ViewMode)
	}
	themes, _ := theme.Load(s.themesDir)
	for _, t := range themes {
		page.Themes = append(page.Themes, views.ThemeOption{ID: t.ID, Name: t.Name})
	}
	ctx := views.WithPage(r.Context(), page)
	for _, c := range cs {
		if err := c.Render(ctx, w); err != nil {
			slog.Error("render", "path", r.URL.Path, "err", err)
			return
		}
	}
}

// fail renders err with a status derived from its kind.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, codehost.ErrNotFound), errors.Is(err, repostore.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, navigator.ErrNoServer), errors.Is(err, installer.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, codehost.ErrOffline):
		status = http.StatusServiceUnavailable
	case errors.Is(err, review.ErrThreadNotFound):
		status = http.StatusNotFound
	case errors.Is(err, review.ErrCannotComment), errors.Is(err, review.ErrNotAtHead), errors.Is(err, servers.ErrBusy):
		status = http.StatusConflict
	case errors.Is(err, review.ErrInvalidComparison), errors.Is(err, review.ErrInvalidReaction),
		errors.Is(err, review.ErrInvalidEvent), errors.Is(err, review.ErrBadPath), errors.Is(err, errBadRequest):
		status = http.StatusBadRequest
	default:
		slog.Error("request failed", "path", r.URL.Path, "err", err)
	}
	if r.Header.Get("HX-Request") == "true" {
		http.Error(w, err.Error(), status)
		return
	}
	s.render(w, r, status, views.Error(status, err.Error()))
}
