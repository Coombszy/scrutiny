package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/Coombszy/scrutiny/internal/cache"
)

func TestRenderMarkdownUsesGFMInTheRepo(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/markdown" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<p><strong>hi</strong> <a href=\"#\">#1</a></p>\n"))
	}))
	t.Cleanup(srv.Close)

	html, err := newClient(srv, cache.NewMemory()).RenderMarkdown(context.Background(), pr325.Repo, "**hi** #1")
	if err != nil {
		t.Fatal(err)
	}
	if html != "<p><strong>hi</strong> <a href=\"#\">#1</a></p>\n" {
		t.Errorf("html = %q", html)
	}
	if want := map[string]string{"text": "**hi** #1", "mode": "gfm", "context": "cli/go-gh"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sent %v, want %v", got, want)
	}
}
