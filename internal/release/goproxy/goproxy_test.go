package goproxy_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Coombszy/scrutiny/internal/release/goproxy"
)

func TestLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Upper case is escaped as !lower in module paths.
		if r.URL.Path != "/github.com/!coombszy/scrutiny/@latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"Version":"v0.3.0","Time":"2026-11-12T09:30:00Z"}`))
	}))
	defer srv.Close()

	s := goproxy.New(srv.URL, "github.com/Coombszy/scrutiny")
	v, err := s.Latest(context.Background())
	if err != nil || v != "v0.3.0" {
		t.Fatalf("Latest() = %q, %v; want v0.3.0", v, err)
	}
}

func TestLatestFailsOnError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if _, err := goproxy.New(srv.URL, "github.com/Coombszy/scrutiny").Latest(context.Background()); err == nil {
		t.Fatal("Latest() succeeded on a 404")
	}
}

func TestLatestWithoutProxy(t *testing.T) {
	_, err := goproxy.New("direct", "github.com/Coombszy/scrutiny").Latest(context.Background())
	if !errors.Is(err, goproxy.ErrNoProxy) {
		t.Fatalf("Latest() err = %v, want ErrNoProxy", err)
	}
}

func TestFirstProxy(t *testing.T) {
	for in, want := range map[string]string{
		"":                                   goproxy.DefaultProxy,
		"https://proxy.golang.org,direct":    "https://proxy.golang.org",
		"https://goproxy.io/|https://b.test": "https://goproxy.io",
		"direct":                             "",
		"off":                                "",
		"direct,https://proxy.golang.org":    "",
	} {
		if got := goproxy.FirstProxy(in); got != want {
			t.Errorf("FirstProxy(%q) = %q, want %q", in, got, want)
		}
	}
}
