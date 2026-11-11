// Package views holds the Templ components for the UI.
package views

import (
	"context"
	"encoding/json"
	"github.com/Coombszy/scrutiny/internal/review"
)

// Page is per-request data every page needs.
type Page struct {
	CSRFToken string
	// ShowLanguages puts the Language Server icon in the top bar, for
	// pages of a PR; Languages is what it shows.
	ShowLanguages bool
	Languages     []review.LanguageStatus
	// Refresh puts the Refresh banner in the top bar, for pages the live
	// stream updates, so it is seen however far down the page is scrolled.
	Refresh bool
	Theme         string
	Themes        []ThemeOption
	Keys          map[string]string // action -> key sequence
	ViewMode      string
	// KeyHelp offers the keyboard shortcuts in the top bar; only pages with
	// hotkeys to show (the PR, file and Dashboard pages) set it. CodeHelp adds the
	// mouse and editing shortcuts of pages showing code.
	KeyHelp, CodeHelp bool
	// AuthFailed opens the dialog saying GitHub rejected the credentials.
	AuthFailed bool
}

// ThemeOption is a Theme the Command Palette can switch to.
type ThemeOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// taglines are typed out, one after another, after the brand in the top bar.
var taglines = []string{
	"GitHub is down... again.",
	"F**k GitHub.",
	"Stupid Buggy GitHub Diff page.",
	"Why is GitHub S**t.",
	"UI/UX is Dead.",
	"Do GitHub even test their own product.",
	"Who wins, This clanked tool or a multi-million dollar company?",
	"It's a Damn Mutiny!",
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
