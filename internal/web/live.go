package web

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Coombszy/scrutiny/internal/domain"
	"github.com/Coombszy/scrutiny/internal/review"
	"github.com/Coombszy/scrutiny/internal/web/views"
)

// heartbeat keeps idle event streams from being closed by intermediaries.
const heartbeat = 25 * time.Second

// events streams a PR's Updates as server-sent events of out-of-band HTML.
// The page holds the stream open only while it is visible.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	ref, ok := prRef(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	updates, leave := s.hub.Subscribe(ref)
	defer leave()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	// A page from before a push (or hidden while another page saw it) is
	// told straight away.
	if head := domain.SHA(r.URL.Query().Get("head")); head != "" {
		if anchors, err := s.review.Anchors(r.Context(), ref); err == nil && anchors.Head != head {
			s.writeUpdate(w, r, ref, review.Update{Head: anchors.Head})
			flusher.Flush()
		}
	}

	ping := time.NewTicker(heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if s.review.Status().Unauthorized {
				// Polling has failed since; the page opens its dialog.
				fmt.Fprint(w, "event: unauthorized\ndata:\n\n")
			} else {
				fmt.Fprint(w, ": ping\n\n")
			}
		case u := <-updates:
			s.writeUpdate(w, r, ref, u)
		}
		flusher.Flush()
	}
}

// writeUpdate writes u as an `update` event of out-of-band HTML.
func (s *Server) writeUpdate(w http.ResponseWriter, r *http.Request, ref domain.PRRef, u review.Update) {
	var html bytes.Buffer
	ctx := views.WithPage(r.Context(), views.Page{CSRFToken: s.csrfToken})
	if err := views.LiveUpdate(ref, u).Render(ctx, &html); err != nil {
		return
	}
	fmt.Fprint(w, "event: update\n")
	for _, line := range strings.Split(html.String(), "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}
