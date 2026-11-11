package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
	s.render(w, r, http.StatusOK, views.SettingsPage(views.SettingsForm{Settings: cfg.WithKeyDefaults()}, list, s.activity(r)))
}

// activityPanel re-renders the Settings page's Activity panel.
func (s *Server) activityPanel(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, views.ActivityPanel(s.activity(r)))
}

func (s *Server) activity(r *http.Request) views.Activity {
	a := views.Activity{RateLimit: s.review.Status().RateLimit}
	if s.metrics != nil {
		if series, err := s.metrics.Summary(r.Context()); err == nil {
			a.Series = series
		}
	}
	return a
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

// newTheme renders the Theme Editor for a new Theme. Its colour pickers
// start from the Theme chosen to copy from, or else the current one.
func (s *Server) newTheme(w http.ResponseWriter, r *http.Request) {
	ed := views.ThemeEditor{Name: r.URL.Query().Get("name"), Paste: true}
	if from, ok := s.copyFrom(r, &ed); ok && ed.Name == "" {
		ed.Name = from.Name + " copy"
	} else if !ok {
		base := s.findTheme(s.currentTheme())
		ed.Scheme, ed.UI, ed.Syntax = base.Scheme, base.UI, base.Syntax
	}
	s.render(w, r, http.StatusOK, views.ThemeEditorView(ed))
}

// editTheme renders the Theme Editor for a Theme in the themes directory,
// or with its colours replaced by the Theme chosen to copy from; built-in
// Themes cannot be edited.
func (s *Server) editTheme(w http.ResponseWriter, r *http.Request) {
	t, err := s.customTheme(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	src, _ := os.ReadFile(theme.File(s.themesDir, t.ID))
	ed := views.ThemeEditor{ID: t.ID, Name: t.Name, Scheme: t.Scheme, Source: string(src), UI: t.UI, Syntax: t.Syntax}
	if name := r.URL.Query().Get("name"); name != "" {
		ed.Name = name
	}
	s.copyFrom(r, &ed)
	s.render(w, r, http.StatusOK, views.ThemeEditorView(ed))
}

// copyFrom fills ed's colours from the Theme the request names in "from",
// opening the colour pickers; ok is false when it names none.
func (s *Server) copyFrom(r *http.Request, ed *views.ThemeEditor) (from theme.Theme, ok bool) {
	themes, _ := theme.Load(s.themesDir)
	from, ok = theme.Find(themes, r.URL.Query().Get("from"))
	if ok {
		ed.From, ed.Paste = from.ID, false
		ed.Scheme, ed.UI, ed.Syntax, ed.Source = from.Scheme, from.UI, from.Syntax, from.TOML()
	}
	return from, ok
}

// findTheme is the Theme id, or the default when there is none.
func (s *Server) findTheme(id string) theme.Theme {
	themes, _ := theme.Load(s.themesDir)
	if t, ok := theme.Find(themes, id); ok {
		return t
	}
	t, _ := theme.Find(themes, theme.Default)
	return t
}

// deleteTheme removes a Theme's file from the themes directory, switching
// to the default Theme if it was the current one.
func (s *Server) deleteTheme(w http.ResponseWriter, r *http.Request) {
	t, err := s.customTheme(r.PathValue("id"))
	if err == nil {
		err = theme.Delete(s.themesDir, t.ID)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.updateSettings(w, r, func(cfg *settings.Settings) {
		if cfg.Theme == t.ID {
			cfg.Theme = theme.Default
		}
	}, true)
}

func (s *Server) customTheme(id string) (theme.Theme, error) {
	themes, _ := theme.Load(s.themesDir)
	if t, ok := theme.Find(themes, id); ok && t.Custom {
		return t, nil
	}
	return theme.Theme{}, fmt.Errorf("%w: no theme %q to edit", theme.ErrNotFound, id)
}

func (s *Server) currentTheme() string {
	cfg, _ := s.settings.Load()
	return cfg.Theme
}

// createTheme saves a new Theme to the themes directory and switches to it.
// What cannot be saved is shown in the Theme Editor, so it stays editable.
func (s *Server) createTheme(w http.ResponseWriter, r *http.Request) {
	t, err := themeFromForm(r)
	if err == nil {
		if themes, _ := theme.Load(s.themesDir); hasTheme(themes, t.ID) {
			err = fmt.Errorf("there is already a theme %q", t.ID)
		} else if s.themesDir == "" {
			err = errors.New("there is no themes directory")
		} else {
			err = theme.Create(s.themesDir, t)
		}
	}
	s.savedTheme(w, r, t.ID, err)
}

// updateTheme saves the Theme Editor over a Theme in the themes directory,
// keeping its file name when it is renamed, and switches to it.
func (s *Server) updateTheme(w http.ResponseWriter, r *http.Request) {
	old, err := s.customTheme(r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := themeFromForm(r)
	if err == nil {
		t.ID = old.ID
		err = theme.Update(s.themesDir, t)
	}
	s.savedTheme(w, r, old.ID, err)
}

func (s *Server) savedTheme(w http.ResponseWriter, r *http.Request, id string, err error) {
	if err != nil {
		s.render(w, r, http.StatusOK, views.ThemeEditorError(err.Error()))
		return
	}
	s.updateSettings(w, r, func(cfg *settings.Settings) { cfg.Theme = id }, true)
}

// themeFromForm is the Theme the Theme Editor describes: imported from its
// pasted colour scheme, or made from its colour pickers.
func themeFromForm(r *http.Request) (theme.Theme, error) {
	name := r.PostFormValue("name")
	if r.PostFormValue("mode") == "paste" {
		return theme.Import(name, r.PostFormValue("source"))
	}
	t := theme.Theme{ID: theme.ID(name), Name: strings.TrimSpace(name), Scheme: r.PostFormValue("scheme"), UI: map[string]string{}, Syntax: map[string]string{}}
	if t.ID == "" {
		return t, errors.New("the name needs a letter or digit")
	}
	ui, syntax := theme.Keys()
	for section, keys := range map[string][]string{"ui": ui, "syntax": syntax} {
		dst := t.UI
		if section == "syntax" {
			dst = t.Syntax
		}
		for _, k := range keys {
			v := strings.TrimSpace(r.PostFormValue(section + "." + k))
			if v == "" {
				continue
			}
			if a, err := strconv.ParseFloat(r.PostFormValue("alpha."+section+"."+k), 64); err == nil {
				v = theme.WithAlpha(v, a)
			}
			dst[k] = v
		}
	}
	return t, nil
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
