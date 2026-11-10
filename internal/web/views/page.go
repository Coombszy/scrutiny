// Package views holds the Templ components for the UI.
package views

import "context"

// Page is per-request data every page needs.
type Page struct {
	CSRFToken string
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
