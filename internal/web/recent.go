package web

import "net/http"

// forgetPR removes the PR from the Dashboard's recently opened list and
// drops its cached data. The empty response replaces the list item.
func (s *Server) forgetPR(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.review.ForgetPR(r.Context(), ref); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
