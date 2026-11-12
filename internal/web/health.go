package web

import "net/http"

// health tells the Userscript that Scrutiny is running on this port.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"app":"scrutiny"}`))
}
