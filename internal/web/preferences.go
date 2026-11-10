package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Coombszy/scrutiny/internal/settings"
	"github.com/Coombszy/scrutiny/internal/theme"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

func hasTheme(themes []theme.Theme, id string) bool {
	_, ok := theme.Find(themes, id)
	return ok
}

// settingsPage shows every setting and the Language Servers.
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.settings.Load()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	list, err := s.servers.List(r.Context(), "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.SettingsPage(views.SettingsForm{Settings: cfg.WithKeyDefaults()}, list))
}

// saveSettings saves the Settings form. Invalid input is shown in the form
// rather than as an error, so it stays editable; a new Theme reloads the
// page to apply it.
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.settings.Load()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cfg = cfg.WithKeyDefaults()
	oldTheme := cfg.Theme
	form := views.SettingsForm{Settings: cfg}
	if err := s.applySettingsForm(r, &form.Settings); err != nil {
		form.Err = err.Error()
	} else if err := s.settings.Save(form.Settings); err != nil {
		form.Err = err.Error()
	} else {
		form.Saved = true
		if form.Settings.Theme != oldTheme {
			w.Header().Set("HX-Refresh", "true")
		}
	}
	s.render(w, r, http.StatusOK, views.SettingsFormView(form))
}

// applySettingsForm copies the posted fields onto cfg and validates it.
func (s *Server) applySettingsForm(r *http.Request, cfg *settings.Settings) error {
	cfg.Theme = r.PostFormValue("theme")
	if themes, _ := theme.Load(s.themesDir); !hasTheme(themes, cfg.Theme) {
		return fmt.Errorf("no theme %q", cfg.Theme)
	}
	cfg.ViewMode = settings.ViewMode(r.PostFormValue("view_mode"))
	for name, dst := range map[string]*int{"collapse_threshold": &cfg.Diff.CollapseThreshold, "port": &cfg.Port} {
		n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue(name)))
		if err != nil {
			return fmt.Errorf("%s must be a whole number", name)
		}
		*dst = n
	}
	for action := range cfg.Keys {
		if v, ok := r.PostForm["key."+action]; ok {
			cfg.Keys[action] = strings.Join(strings.Fields(v[0]), " ")
		}
	}
	return cfg.Validate()
}

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
