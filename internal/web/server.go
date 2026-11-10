// Package web is the driving HTTP adapter: it serves the Templ + htmx UI.
package web

import (
	"context"
	"io"
	"io/fs"
	"net/http"

	"github.com/Coombszy/scrutiny/internal/web/views"
)

// Server serves the UI.
type Server struct {
	csrfToken string
	mux       *http.ServeMux
}

// NewServer returns a Server whose mutating requests must carry csrfToken.
func NewServer(csrfToken string) *Server {
	s := &Server{csrfToken: csrfToken, mux: http.NewServeMux()}
	static, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		s.render(w, r, views.Home())
	})
	return s
}

// Handler returns the root handler with the security middleware applied.
func (s *Server) Handler() http.Handler {
	return Secure(s.mux, s.csrfToken)
}

type component interface {
	Render(ctx context.Context, w io.Writer) error
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, c component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	ctx := views.WithPage(r.Context(), views.Page{CSRFToken: s.csrfToken})
	if err := c.Render(ctx, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
