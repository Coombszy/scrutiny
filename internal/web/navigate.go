package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

func (s *Server) routeNavigation() {
	pr := "/github.com/{owner}/{repo}/pull/{number}"
	s.mux.HandleFunc("GET "+pr+"/definition", s.definition)
	s.mux.HandleFunc("GET "+pr+"/references", s.references)
	s.mux.HandleFunc("GET "+pr+"/symbols", s.symbols)
	s.mux.HandleFunc("GET "+pr+"/blob", s.blob)
}

// navPosition reads head (the commit the page shows), path, line and col
// (a UTF-16 offset) from the query.
func navPosition(r *http.Request) (domain.SHA, string, navigator.Position, error) {
	q := r.URL.Query()
	line, err1 := strconv.Atoi(q.Get("line"))
	col, err2 := strconv.Atoi(q.Get("col"))
	if q.Get("path") == "" || err1 != nil || err2 != nil || line < 1 || col < 0 || !shaPattern.MatchString(q.Get("head")) {
		return "", "", navigator.Position{}, fmt.Errorf("%w: head, path, line and col are required", errBadRequest)
	}
	return domain.SHA(q.Get("head")), q.Get("path"), navigator.Position{Line: line, Col: col}, nil
}

type targetJSON struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	InPR    bool   `json:"inPR"`
	Outside bool   `json:"outside"`
	URL     string `json:"url"`
}

// definition answers JSON, since the client decides whether to scroll the
// diff or open the file view.
func (s *Server) definition(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	head, path, pos, err := navPosition(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	targets, err := s.review.Definition(r.Context(), ref, head, path, pos)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]targetJSON, len(targets))
	for i, t := range targets {
		out[i] = targetJSON{Path: t.Path, Line: t.Line, Col: t.Col, InPR: t.InPR, Outside: t.OutsideProject}
		if !t.OutsideProject {
			out[i].URL = views.BlobURL(ref, t.Path, t.Line)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) references(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	head, path, pos, err := navPosition(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	refs, err := s.review.References(r.Context(), ref, head, path, pos)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ReferencesPanel(ref, refs))
}

func (s *Server) symbols(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	results, err := s.review.SymbolSearch(r.Context(), ref, r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.SymbolResults(ref, results))
}

func (s *Server) blob(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		s.fail(w, r, fmt.Errorf("%w: path is required", errBadRequest))
		return
	}
	fv, err := s.review.FileView(r.Context(), ref, path)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderPage(w, r, http.StatusOK, views.Page{ShowLanguages: true, Languages: s.review.LanguageStatus(r.Context(), ref)}, views.FileViewPage(ref, fv))
}
