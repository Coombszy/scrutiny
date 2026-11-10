// Package views holds the Templ components for the UI.
package views

import (
	"context"
	"encoding/json"
)

// Page is per-request data every page needs.
type Page struct {
	CSRFToken string
	Indexing  bool // a Language Server for this page's PR is Indexing
	Theme     string
	Themes    []ThemeOption
	Keys      map[string]string // action -> key sequence
	ViewMode  string
	// KeyHelp offers the keyboard shortcuts in the top bar; only pages with
	// hotkeys to show (the PR page and Dashboard) set it. CodeHelp adds the
	// mouse and editing shortcuts of pages showing code.
	KeyHelp, CodeHelp bool
}

// ThemeOption is a Theme the Command Palette can switch to.
type ThemeOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type pageKey struct{}

// WithPage stores p in ctx for components to read.
func WithPage(ctx context.Context, p Page) context.Context {
	return context.WithValue(ctx, pageKey{}, p)
}

func pageFrom(ctx context.Context) Page {
	p, _ := ctx.Value(pageKey{}).(Page)
	return p
}

func csrfHeaders(ctx context.Context) string {
	return `{"X-CSRF-Token":"` + pageFrom(ctx).CSRFToken + `"}`
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
