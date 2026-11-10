package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/theme"
)

// themeCSS serves the configured Theme, falling back to the default when it
// is unknown (e.g. its file was removed).
func (s *Server) themeCSS(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.settings.Load()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	themes, errs := theme.Load(s.themesDir)
	for _, err := range errs {
		slog.Warn("skipping theme", "err", err)
	}
	t, ok := theme.Find(themes, cfg.Theme)
	if !ok {
		t, _ = theme.Find(themes, theme.Default)
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, t.CSS())
}

// setTheme saves the Theme to the config file and reloads the page.
func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	id := r.PostFormValue("theme")
	themes, _ := theme.Load(s.themesDir)
	if _, ok := theme.Find(themes, id); !ok {
		s.fail(w, r, fmt.Errorf("%w: no theme %q", errBadRequest, id))
		return
	}
	s.updateSettings(w, r, func(cfg *settings.Settings) { cfg.Theme = id }, true)
}

// setKey rebinds one action's hotkey.
func (s *Server) setKey(w http.ResponseWriter, r *http.Request) {
	action, binding := r.PostFormValue("action"), r.PostFormValue("binding")
	s.updateSettings(w, r, func(cfg *settings.Settings) { cfg.Keys[action] = binding }, false)
}

// updateSettings applies change to the config file, rejecting it if the
// result is invalid, and optionally reloads the page.
func (s *Server) updateSettings(w http.ResponseWriter, r *http.Request, change func(*settings.Settings), reload bool) {
	cfg, err := s.settings.Load()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cfg = cfg.WithKeyDefaults()
	change(&cfg)
	if err := cfg.Validate(); err != nil {
		s.fail(w, r, fmt.Errorf("%w: %v", errBadRequest, err))
		return
	}
	if err := s.settings.Save(cfg); err != nil {
		s.fail(w, r, err)
		return
	}
	if reload {
		w.Header().Set("HX-Refresh", "true")
	}
	w.WriteHeader(http.StatusNoContent)
}
