// Package web is the driving HTTP adapter: it serves the Templ + htmx UI.
package web

import (
	"io/fs"
	"net/http"

	"github.com/Coombszy/scrutiny/internal/web/views"
)

// NewHandler returns the root HTTP handler for the UI.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		_ = views.Home().Render(r.Context(), w)
	})
	return mux
}
