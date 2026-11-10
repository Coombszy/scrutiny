package lsp_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/navigator/lsp"
)

// gopls finds gopls via $SCRUTINY_GOPLS or $PATH, skipping when absent.
func gopls(t *testing.T) string {
	t.Helper()
	if p := os.Getenv("SCRUTINY_GOPLS"); p != "" {
		return p
	}
	p, err := exec.LookPath("gopls")
	if err != nil {
		t.Skip("gopls not found; set SCRUTINY_GOPLS to run Language Server tests")
	}
	return p
}

// module writes a small Go module: greet.go declares Greet (line 4) and
// main.go calls it (line 6, column 7).
func module(t *testing.T) navigator.Workspace {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":   "module example.com/m\n\ngo 1.22\n",
		"greet.go": "package main\n\n// Greet says hello.\nfunc Greet(name string) string {\n\treturn \"hello \" + name\n}\n",
		"main.go":  "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(Greet(\"you\"))\n}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return navigator.Workspace{Root: dir, Language: "go"}
}

func newManager(t *testing.T, idle time.Duration) *lsp.Manager {
	t.Helper()
	m := lsp.NewManager(map[string][]string{"go": {gopls(t)}}, idle)
	t.Cleanup(m.Close)
	return m
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestDefinitionAcrossFiles(t *testing.T) {
	m, ws := newManager(t, time.Minute), module(t)

	got, err := m.Definition(ctx(t), ws, "main.go", navigator.Position{Line: 6, Col: 14})
	if err != nil {
		t.Fatal(err)
	}
	want := navigator.Location{Path: "greet.go", Line: 4, Col: 5, EndCol: 10}
	if len(got) != 1 || got[0] != want {
		t.Errorf("Definition = %+v, want [%+v]", got, want)
	}
}

func TestDefinitionOutsideTheProject(t *testing.T) {
	m, ws := newManager(t, time.Minute), module(t)

	got, err := m.Definition(ctx(t), ws, "main.go", navigator.Position{Line: 6, Col: 6})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].OutsideProject || filepath.Base(got[0].Path) != "print.go" {
		t.Errorf("Definition of fmt.Println = %+v, want print.go outside the project", got)
	}
}

func TestReferencesIncludeTheCallSite(t *testing.T) {
	m, ws := newManager(t, time.Minute), module(t)

	got, err := m.References(ctx(t), ws, "greet.go", navigator.Position{Line: 4, Col: 6})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got, func(l navigator.Location) bool { return l.Path == "main.go" && l.Line == 6 }) {
		t.Errorf("References = %+v, want main.go:6", got)
	}
}

func TestSymbolsMatchAcrossTheWorkspace(t *testing.T) {
	m, ws := newManager(t, time.Minute), module(t)

	got, err := m.Symbols(ctx(t), ws, "Greet")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(got, func(s navigator.Symbol) bool {
		return s.Name == "Greet" && s.Kind == "function" && s.Path == "greet.go" && s.Line == 4
	}) {
		t.Errorf("Symbols = %+v, want Greet in greet.go:4", got)
	}
}

func TestIndexingIsReportedUntilTheServerIsReady(t *testing.T) {
	m, ws := newManager(t, time.Minute), module(t)
	var (
		mu     sync.Mutex
		states []bool
	)
	m.OnIndexing(func(w navigator.Workspace, indexing bool) {
		mu.Lock()
		defer mu.Unlock()
		if w == ws {
			states = append(states, indexing)
		}
	})
	if m.Indexing(ws) {
		t.Error("Indexing before any request")
	}

	if _, err := m.Definition(ctx(t), ws, "main.go", navigator.Position{Line: 6, Col: 14}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(states) < 2 || !states[0] || states[len(states)-1] {
		t.Errorf("indexing transitions = %v, want true … false", states)
	}
	if m.Indexing(ws) {
		t.Error("still Indexing after answering")
	}
}

func TestIdleServersStopAndRestartOnDemand(t *testing.T) {
	m, ws := newManager(t, 200*time.Millisecond), module(t)
	if _, err := m.Definition(ctx(t), ws, "main.go", navigator.Position{Line: 6, Col: 14}); err != nil {
		t.Fatal(err)
	}
	if !m.Running(ws) {
		t.Fatal("server not running after a request")
	}

	deadline := time.Now().Add(10 * time.Second)
	for m.Running(ws) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if m.Running(ws) {
		t.Fatal("idle server was not stopped")
	}
	if _, err := m.Definition(ctx(t), ws, "main.go", navigator.Position{Line: 6, Col: 14}); err != nil {
		t.Errorf("after restart: %v", err)
	}
}

func TestUnknownLanguageHasNoServer(t *testing.T) {
	m := lsp.NewManager(map[string][]string{}, time.Minute)
	defer m.Close()
	_, err := m.Definition(context.Background(), navigator.Workspace{Root: t.TempDir(), Language: "cobol"}, "x.cbl", navigator.Position{Line: 1})
	if err == nil || !errors.Is(err, navigator.ErrNoServer) {
		t.Errorf("err = %v, want ErrNoServer", err)
	}
}
