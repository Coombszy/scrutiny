package web

import (
	"errors"
	"net/http"

	"github.com/Coombszy/scrutiny/internal/installer"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

func (s *Server) routeServers() {
	s.mux.HandleFunc("GET /settings/servers", s.serversPage)
	s.mux.HandleFunc("GET /settings/servers/list", s.serverList)
	s.mux.HandleFunc("POST /settings/servers/refresh", s.refreshServers)
	s.mux.HandleFunc("GET /settings/servers/{name}", s.serverRow)
	s.mux.HandleFunc("POST /settings/servers/{name}/install", s.installServer)
	s.mux.HandleFunc("POST /settings/servers/{name}/remove", s.removeServer)
}

func (s *Server) serversPage(w http.ResponseWriter, r *http.Request) {
	list, err := s.servers.List(r.Context(), "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ServersPage(list))
}

func (s *Server) serverList(w http.ResponseWriter, r *http.Request) {
	list, err := s.servers.List(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ServerList(list))
}

func (s *Server) refreshServers(w http.ResponseWriter, r *http.Request) {
	if err := s.servers.Refresh(r.Context()); err != nil {
		s.fail(w, r, err)
		return
	}
	s.serverList(w, r)
}

func (s *Server) serverRow(w http.ResponseWriter, r *http.Request) {
	st, err := s.servers.Status(r.Context(), r.PathValue("name"))
	if errors.Is(err, installer.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.ServerRow(st))
}

func (s *Server) installServer(w http.ResponseWriter, r *http.Request) {
	if _, err := s.servers.Status(r.Context(), r.PathValue("name")); err != nil {
		s.serverRow(w, r)
		return
	}
	s.servers.Install(r.PathValue("name"))
	s.serverRow(w, r)
}

func (s *Server) removeServer(w http.ResponseWriter, r *http.Request) {
	if err := s.servers.Remove(r.PathValue("name")); err != nil {
		s.fail(w, r, err)
		return
	}
	s.serverRow(w, r)
}
