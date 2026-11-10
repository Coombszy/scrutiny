package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/Coombszy/scrutiny/internal/codehost"
	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

func (s *Server) routeReview() {
	pr := "/github.com/{owner}/{repo}/pull/{number}"
	s.mux.HandleFunc("GET "+pr+"/threads/new", s.newThreadForm)
	s.mux.HandleFunc("POST "+pr+"/threads", s.createThread)
	s.mux.HandleFunc("POST "+pr+"/threads/{thread}/replies", s.reply)
	s.mux.HandleFunc("POST "+pr+"/threads/{thread}/resolve", s.resolve)
	s.mux.HandleFunc("POST "+pr+"/reactions", s.react)
	s.mux.HandleFunc("POST "+pr+"/comments", s.comment)
	s.mux.HandleFunc("POST "+pr+"/viewed", s.viewed)
	s.mux.HandleFunc("POST "+pr+"/review", s.submit)
}

// threadAnchor reads a new thread's anchor from the query or form.
func threadAnchor(r *http.Request) (views.ThreadAnchor, int, error) {
	a := views.ThreadAnchor{
		Base: r.FormValue("base"), Head: r.FormValue("head"), Path: r.FormValue("path"), Side: r.FormValue("side"),
	}
	line, err := strconv.Atoi(r.FormValue("line"))
	if err != nil || line < 1 || !shaPattern.MatchString(a.Base) || !shaPattern.MatchString(a.Head) || a.Path == "" ||
		(a.Side != string(domain.Left) && a.Side != string(domain.Right)) {
		return a, 0, fmt.Errorf("%w: base, head, path, line and side are required", errBadRequest)
	}
	a.Line = line
	cols := 4
	if r.FormValue("cols") == "3" {
		cols = 3
	}
	return a, cols, nil
}

func (s *Server) newThreadForm(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	a, cols, err := threadAnchor(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.NewThreadForm(ref, a, cols))
}

func (s *Server) createThread(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	a, cols, err := threadAnchor(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	threads, err := s.review.AddThread(r.Context(), ref, domain.SHA(a.Base), domain.SHA(a.Head), codehost.NewThread{
		Path: a.Path, Line: a.Line, Side: domain.DiffSide(a.Side), Body: r.FormValue("body"),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderWithPending(w, r, views.ThreadRows(ref, threads, cols))
}

func (s *Server) reply(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := s.review.Reply(r.Context(), ref, r.PathValue("thread"), r.FormValue("body"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.renderWithPending(w, r, views.Thread(ref, t))
}

func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	t, err := s.review.SetResolved(r.Context(), ref, r.PathValue("thread"), r.FormValue("resolved") == "true")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.Thread(ref, t))
}

func (s *Server) react(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	subject := r.FormValue("subject")
	groups, err := s.review.SetReaction(r.Context(), ref, subject, domain.Reaction(r.FormValue("content")), r.FormValue("on") == "true")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.Reactions(ref, subject, groups))
}

func (s *Server) comment(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	comments, err := s.review.AddConversationComment(r.Context(), ref, r.FormValue("body"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.Conversation(ref, comments))
}

func (s *Server) viewed(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.review.SetViewed(r.Context(), ref, r.FormValue("path"), r.FormValue("viewed") == "true"); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) submit(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	head := r.FormValue("head")
	if !shaPattern.MatchString(head) {
		s.fail(w, r, fmt.Errorf("%w: head is required", errBadRequest))
		return
	}
	err := s.review.Submit(r.Context(), ref, domain.SHA(head), domain.ReviewEvent(r.FormValue("event")), r.FormValue("body"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("HX-Refresh", "true")
	w.WriteHeader(http.StatusNoContent)
}

// renderWithPending renders c followed by out-of-band updates of the
// Pending Review's count and list.
func (s *Server) renderWithPending(w http.ResponseWriter, r *http.Request, c component) {
	ref := prRefOrZero(r)
	anchors, err := s.review.Anchors(r.Context(), ref)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pending, err := s.review.PendingComments(r.Context(), ref)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, c, views.PendingCount(anchors.Pending, true), views.PendingList(pending, true))
}

func prRefOrZero(r *http.Request) domain.PRRef {
	ref, _ := prRef(r)
	return ref
}
