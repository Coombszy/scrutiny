package lsp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Coombszy/scrutiny/internal/navigator"
	"github.com/Coombszy/scrutiny/internal/navigator/lsp"
)

// fakeMode, when set, makes the test binary a fake Language Server instead.
const fakeMode = "SCRUTINY_FAKE_LSP"

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeMode); mode != "" {
		fakeServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeServer speaks just enough LSP over stdio. Its mode is one of:
//
//	nosymbols   no workspace symbol search; logs an error and a stderr line
//	symbolerror workspace symbol search answers with an error
//	crash       exits with status 3 once initialised
func fakeServer(mode string) {
	in := bufio.NewReader(os.Stdin)
	tp := textproto.NewReader(in)
	send := func(m map[string]any) {
		m["jsonrpc"] = "2.0"
		body, _ := json.Marshal(m)
		_, _ = fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	for {
		header, err := tp.ReadMIMEHeader()
		if err != nil {
			return
		}
		n, _ := strconv.Atoi(header.Get("Content-Length"))
		body := make([]byte, n)
		if _, err := io.ReadFull(in, body); err != nil {
			return
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.Unmarshal(body, &msg)
		switch msg.Method {
		case "initialize":
			caps := map[string]any{}
			if mode != "nosymbols" {
				caps["workspaceSymbolProvider"] = true
			}
			send(map[string]any{"id": msg.ID, "result": map[string]any{"capabilities": caps}})
		case "initialized":
			switch mode {
			case "nosymbols":
				fmt.Fprintln(os.Stderr, "fake: loading schemas")
				send(map[string]any{"method": "window/logMessage", "params": map[string]any{"type": 1, "message": "schema download failed"}})
			case "crash":
				fmt.Fprintln(os.Stderr, "fake: fatal error")
				os.Exit(3)
			}
		case "workspace/symbol":
			send(map[string]any{"id": msg.ID, "error": map[string]any{"code": -32601, "message": "Unhandled method workspace/symbol"}})
		case "shutdown":
			send(map[string]any{"id": msg.ID, "result": nil})
		case "exit":
			return
		}
	}
}

// fakeManager runs the fake server in mode for "fake" Workspaces.
func fakeManager(t *testing.T, mode string) (*lsp.Manager, navigator.Workspace, string) {
	t.Helper()
	t.Setenv(fakeMode, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := lsp.NewManager(lsp.Static(map[string][]string{"fake": {exe}}), time.Minute)
	t.Cleanup(m.Close)
	return m, navigator.Workspace{Root: t.TempDir(), Language: "fake"}, exe
}

// waitFor polls until ok holds of the Workspace's state, or fails.
func waitFor(t *testing.T, m *lsp.Manager, ws navigator.Workspace, ok func(navigator.ServerState) bool) navigator.ServerState {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		st := m.State(ws)
		if ok(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatalf("state = %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func phase(p navigator.Phase) func(navigator.ServerState) bool {
	return func(st navigator.ServerState) bool { return st.Phase == p }
}

func TestAServerWithoutSymbolSearchBecomesReady(t *testing.T) {
	m, ws, exe := fakeManager(t, "nosymbols")
	if err := m.Warm(ctx(t), ws); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, ws, phase(navigator.Ready))
	st := waitFor(t, m, ws, func(st navigator.ServerState) bool { return st.LastProblem != "" })
	if st.LastProblem != "schema download failed" {
		t.Errorf("LastProblem = %q", st.LastProblem)
	}

	var stderr, logged bool
	deadline := time.Now().Add(5 * time.Second)
	for (!stderr || !logged) && time.Now().Before(deadline) {
		log := m.Log(exe)
		stderr = slices.ContainsFunc(log, func(e navigator.LogEntry) bool {
			return e.Source == "stderr" && e.Text == "fake: loading schemas"
		})
		logged = slices.ContainsFunc(log, func(e navigator.LogEntry) bool {
			return e.Source == "server" && e.Level == navigator.LogError && e.Text == "schema download failed"
		})
		time.Sleep(20 * time.Millisecond)
	}
	if !stderr || !logged {
		t.Errorf("log = %+v, want its stderr line and logMessage", m.Log(exe))
	}
}

func TestAWarmUpAnsweredWithAnErrorIsReady(t *testing.T) {
	m, ws, exe := fakeManager(t, "symbolerror")
	if err := m.Warm(ctx(t), ws); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, ws, phase(navigator.Ready))
	if !slices.ContainsFunc(m.Log(exe), func(e navigator.LogEntry) bool {
		return e.Level == navigator.LogWarn && strings.Contains(e.Text, "Unhandled method")
	}) {
		t.Errorf("log = %+v, want the warm-up's error", m.Log(exe))
	}
}

func TestAServerThatExitsIsFailedWithItsStatus(t *testing.T) {
	m, ws, exe := fakeManager(t, "crash")
	if err := m.Warm(ctx(t), ws); err != nil {
		t.Fatal(err)
	}
	st := waitFor(t, m, ws, phase(navigator.Failed))
	if !strings.Contains(st.Err, "exit status 3") {
		t.Errorf("Err = %q, want the exit status", st.Err)
	}
	log := m.Log(exe)
	if !slices.ContainsFunc(log, func(e navigator.LogEntry) bool { return e.Text == "fake: fatal error" }) {
		t.Errorf("log = %+v, want its stderr", log)
	}
}

func TestAStoppedServerIsNotFailed(t *testing.T) {
	m, ws, exe := fakeManager(t, "symbolerror")
	if err := m.Warm(ctx(t), ws); err != nil {
		t.Fatal(err)
	}
	waitFor(t, m, ws, phase(navigator.Ready))
	m.Reset(ws)
	waitFor(t, m, ws, phase(navigator.Stopped))
	time.Sleep(200 * time.Millisecond) // let the process exit
	if st := m.State(ws); st.Phase != navigator.Stopped {
		t.Errorf("state = %+v, want stopped", st)
	}
	if slices.ContainsFunc(m.Log(exe), func(e navigator.LogEntry) bool { return e.Level == navigator.LogError }) {
		t.Errorf("log = %+v, want no errors", m.Log(exe))
	}
}
