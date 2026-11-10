// Package lsp is the CodeNavigator adapter backed by Language Servers
// speaking LSP over stdio: one server per Workspace, started on first use
// and stopped once idle.
package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/Coombszy/scrutiny/internal/navigator"
)

// Manager runs Language Servers on demand.
type Manager struct {
	command func(language string) []string // nil when no server is available
	idle    time.Duration

	mu        sync.Mutex
	servers   map[navigator.Workspace]*server
	failures  map[navigator.Workspace]navigator.ServerState // the last failed start, until the next try
	onState   func(navigator.Workspace, navigator.ServerState)
	done      chan struct{}
	closeOnce sync.Once
}

var _ navigator.Navigator = (*Manager)(nil)

// NewManager returns a Manager that runs command(language) for a
// Workspace, looked up each time a server starts so newly installed servers
// are picked up, and stops servers unused for idle.
func NewManager(command func(language string) []string, idle time.Duration) *Manager {
	m := &Manager{command: command, idle: idle, servers: map[navigator.Workspace]*server{},
		failures: map[navigator.Workspace]navigator.ServerState{}, done: make(chan struct{})}
	go m.reap()
	return m
}

// Static launches servers from a fixed table of language -> argv.
func Static(commands map[string][]string) func(string) []string {
	return func(language string) []string { return commands[language] }
}

// OnState registers fn to be told whenever a Workspace's server changes
// Phase.
func (m *Manager) OnState(fn func(navigator.Workspace, navigator.ServerState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onState = fn
}

func (m *Manager) Available(language string) bool { return len(m.command(language)) > 0 }

// State reports the Workspace's server: running, failed to start, or
// stopped (it starts on the next query).
func (m *Manager) State(ws navigator.Workspace) navigator.ServerState {
	m.mu.Lock()
	s, ok := m.servers[ws]
	failed, hasFailed := m.failures[ws]
	m.mu.Unlock()
	switch {
	case ok:
		return s.state()
	case hasFailed:
		return failed
	}
	st := navigator.ServerState{Phase: navigator.Stopped}
	if argv := m.command(ws.Language); len(argv) > 0 {
		st.Server = filepath.Base(argv[0])
	}
	return st
}

// Warm starts the Workspace's server without waiting for it, then asks it a
// trivial symbol query: answering marks it Ready even if it never reports
// its indexing progress.
func (m *Manager) Warm(_ context.Context, ws navigator.Workspace) error {
	if !m.Available(ws.Language) {
		return fmt.Errorf("%w for %s", navigator.ErrNoServer, ws.Language)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		s, err := m.server(ctx, ws)
		if err != nil {
			return
		}
		defer s.release()
		var found json.RawMessage
		if s.conn.call(ctx, "workspace/symbol", map[string]string{"query": "scrutiny-warm-up"}, &found) == nil {
			s.answered()
		}
	}()
	return nil
}

// Running reports whether ws has a live server.
func (m *Manager) Running(ws navigator.Workspace) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.servers[ws]
	return ok
}

func (m *Manager) Reset(ws navigator.Workspace) {
	m.mu.Lock()
	s, ok := m.servers[ws]
	delete(m.servers, ws)
	m.mu.Unlock()
	if ok {
		go s.stop()
	}
}

// Close stops every server.
func (m *Manager) Close() {
	m.closeOnce.Do(func() { close(m.done) })
	m.mu.Lock()
	servers := m.servers
	m.servers = map[navigator.Workspace]*server{}
	m.mu.Unlock()
	for _, s := range servers {
		s.stop()
	}
}

func (m *Manager) Definition(ctx context.Context, ws navigator.Workspace, path string, pos navigator.Position) ([]navigator.Location, error) {
	return m.locationRequest(ctx, ws, "textDocument/definition", path, pos, nil)
}

func (m *Manager) References(ctx context.Context, ws navigator.Workspace, path string, pos navigator.Position) ([]navigator.Location, error) {
	return m.locationRequest(ctx, ws, "textDocument/references", path, pos, map[string]bool{"includeDeclaration": true})
}

func (m *Manager) locationRequest(ctx context.Context, ws navigator.Workspace, method, path string, pos navigator.Position, refContext any) ([]navigator.Location, error) {
	s, err := m.server(ctx, ws)
	if err != nil {
		return nil, err
	}
	defer s.release()
	if err := s.open(path); err != nil {
		return nil, err
	}
	params := map[string]any{
		"textDocument": map[string]string{"uri": fileURI(filepath.Join(ws.Root, path))},
		"position":     lspPosition{Line: pos.Line - 1, Character: pos.Col},
	}
	if refContext != nil {
		params["context"] = refContext
	}
	var raw json.RawMessage
	if err := s.conn.call(ctx, method, params, &raw); err != nil {
		return nil, err
	}
	s.answered()
	return locations(ws.Root, raw)
}

func (m *Manager) Symbols(ctx context.Context, ws navigator.Workspace, query string) ([]navigator.Symbol, error) {
	s, err := m.server(ctx, ws)
	if err != nil {
		return nil, err
	}
	defer s.release()
	var found []symbolInformation
	if err := s.conn.call(ctx, "workspace/symbol", map[string]string{"query": query}, &found); err != nil {
		return nil, err
	}
	s.answered()
	out := make([]navigator.Symbol, 0, len(found))
	for _, f := range found {
		out = append(out, navigator.Symbol{
			Name: f.Name, Kind: symbolKinds[f.Kind], Container: f.ContainerName,
			Location: toLocation(ws.Root, f.Location),
		})
	}
	return out, nil
}

// server returns ws's server, starting it if needed, and marks it in use
// until release is called.
func (m *Manager) server(ctx context.Context, ws navigator.Workspace) (*server, error) {
	m.mu.Lock()
	s, ok := m.servers[ws]
	if !ok {
		argv := m.command(ws.Language)
		if len(argv) == 0 {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w for %s", navigator.ErrNoServer, ws.Language)
		}
		s = newServer(ws, argv)
		s.notify = func(st navigator.ServerState) { m.notify(s, st) }
		s.onExit = func() { m.forget(s) }
		m.servers[ws] = s
		delete(m.failures, ws)
		go s.start()
	}
	s.acquire()
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		s.release()
		return nil, ctx.Err()
	case <-s.ready:
	}
	if s.err != nil {
		s.release()
		m.mu.Lock()
		if m.servers[ws] == s {
			delete(m.servers, ws)
			m.failures[ws] = s.state()
		}
		m.mu.Unlock()
		return nil, s.err
	}
	return s, nil
}

// notify reports s's state, unless s has been replaced: a stopping server
// must not overwrite its successor's state.
func (m *Manager) notify(s *server, st navigator.ServerState) {
	m.mu.Lock()
	fn := m.onState
	current, ok := m.servers[s.ws]
	m.mu.Unlock()
	if fn != nil && (!ok || current == s) {
		fn(s.ws, st)
	}
}

// forget drops s once its process has exited, so the next query starts a
// fresh server.
func (m *Manager) forget(s *server) {
	m.mu.Lock()
	if m.servers[s.ws] == s {
		delete(m.servers, s.ws)
	}
	m.mu.Unlock()
	if s.state().Phase != navigator.Failed {
		s.setPhase(navigator.Stopped)
	}
}

// reap stops servers that have been idle for longer than m.idle.
func (m *Manager) reap() {
	tick := time.NewTicker(min(m.idle/4, time.Minute))
	defer tick.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-tick.C:
		}
		m.mu.Lock()
		var idle []*server
		for ws, s := range m.servers {
			if s.idleFor() > m.idle {
				idle = append(idle, s)
				delete(m.servers, ws)
			}
		}
		m.mu.Unlock()
		for _, s := range idle {
			s.stop()
		}
	}
}

// server is one running Language Server process.
type server struct {
	ws     navigator.Workspace
	argv   []string
	notify func(navigator.ServerState)
	onExit func()

	exited   chan struct{} // closed once the process has been waited for
	waitOnce sync.Once

	ready chan struct{} // closed once initialised (or failed: see err)
	err   error
	cmd   *exec.Cmd
	conn  *conn

	mu       sync.Mutex
	inUse    int
	lastUsed time.Time
	opened   map[string]bool
	progress map[string]bool // active $/progress tokens
	phase    navigator.Phase
	errText  string
}

func newServer(ws navigator.Workspace, argv []string) *server {
	return &server{ws: ws, argv: argv, ready: make(chan struct{}), exited: make(chan struct{}),
		lastUsed: time.Now(), opened: map[string]bool{}, progress: map[string]bool{}}
}

// wait reaps the process once, however many callers ask.
func (s *server) wait() {
	s.waitOnce.Do(func() {
		_ = s.cmd.Wait()
		close(s.exited)
	})
}

func (s *server) start() {
	defer close(s.ready)
	s.setPhase(navigator.Starting)
	if err := s.launch(); err != nil {
		s.err = fmt.Errorf("start %s: %w", s.argv[0], err)
		s.mu.Lock()
		s.errText = s.err.Error()
		s.mu.Unlock()
		s.setPhase(navigator.Failed)
		return
	}
	s.setPhase(navigator.Indexing)
}

func (s *server) launch() error {
	s.cmd = exec.Command(s.argv[0], s.argv[1:]...)
	s.cmd.Dir = s.ws.Root
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := s.cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %v", navigator.ErrNoServer, err)
		}
		return err
	}
	s.conn = newConn(stdin)
	s.conn.onNotify = s.handleNotification
	s.conn.onRequest = s.handleRequest
	go s.conn.read(stdout)
	// Reap the process itself, not when stdout closes: a child it spawned
	// may hold stdout open after it exits. Wait then closes our end, ending
	// the reader.
	go func() {
		s.wait()
		if s.onExit != nil {
			s.onExit()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	root := fileURI(s.ws.Root)
	init := map[string]any{
		"processId":        os.Getpid(),
		"clientInfo":       map[string]string{"name": "scrutiny"},
		"rootUri":          root,
		"workspaceFolders": []map[string]string{{"uri": root, "name": filepath.Base(s.ws.Root)}},
		"capabilities": map[string]any{
			"window":       map[string]any{"workDoneProgress": true},
			"workspace":    map[string]any{"workspaceFolders": true, "symbol": map[string]any{}},
			"textDocument": map[string]any{"definition": map[string]any{}, "references": map[string]any{}, "synchronization": map[string]any{}},
		},
	}
	if err := s.conn.call(ctx, "initialize", init, nil); err != nil {
		s.kill()
		<-s.exited
		return err
	}
	if err := s.conn.notify("initialized", struct{}{}); err != nil {
		s.kill()
		<-s.exited
		return err
	}
	return nil
}

func (s *server) handleNotification(method string, params json.RawMessage) {
	if method != "$/progress" {
		return
	}
	var p progressParams
	if json.Unmarshal(params, &p) != nil {
		return
	}
	s.mu.Lock()
	switch p.Value.Kind {
	case "begin":
		s.progress[string(p.Token)] = true
	case "end":
		delete(s.progress, string(p.Token))
	}
	active := len(s.progress) > 0
	starting := s.phase == navigator.Starting
	s.mu.Unlock()
	switch {
	case starting: // still initialising: Indexing follows launch
	case active:
		s.setPhase(navigator.Indexing)
	default:
		s.setPhase(navigator.Ready)
	}
}

func (s *server) handleRequest(method string, params json.RawMessage) (any, error) {
	switch method {
	case "window/workDoneProgress/create", "client/registerCapability", "client/unregisterCapability":
		return nil, nil
	case "workspace/configuration":
		var p struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.Unmarshal(params, &p)
		return make([]any, len(p.Items)), nil
	case "workspace/workspaceFolders":
		return []map[string]string{{"uri": fileURI(s.ws.Root), "name": filepath.Base(s.ws.Root)}}, nil
	}
	return nil, fmt.Errorf("unsupported method %s", method)
}

func (s *server) setPhase(p navigator.Phase) {
	s.mu.Lock()
	changed := s.phase != p
	s.phase = p
	s.mu.Unlock()
	if changed && s.notify != nil {
		s.notify(s.state())
	}
}

func (s *server) state() navigator.ServerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return navigator.ServerState{Phase: s.phase, Server: filepath.Base(s.argv[0]), Err: s.errText}
}

// answered marks the server Ready: it has answered a query.
func (s *server) answered() {
	s.mu.Lock()
	clear(s.progress)
	s.mu.Unlock()
	s.setPhase(navigator.Ready)
}

func (s *server) acquire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inUse++
	s.lastUsed = time.Now()
}

func (s *server) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inUse--
	s.lastUsed = time.Now()
}

func (s *server) idleFor() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inUse > 0 {
		return 0
	}
	return time.Since(s.lastUsed)
}

// open sends didOpen for path once, so the server answers from its content.
func (s *server) open(path string) error {
	s.mu.Lock()
	if s.opened[path] {
		s.mu.Unlock()
		return nil
	}
	s.opened[path] = true
	s.mu.Unlock()
	full := filepath.Join(s.ws.Root, path)
	text, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	return s.conn.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": fileURI(full), "languageId": languageID(s.ws.Language), "version": 1, "text": string(text),
	}})
}

// drainTimeout bounds how long stop waits for in-flight queries.
const drainTimeout = 30 * time.Second

// stop lets in-flight queries finish, asks the server to shut down, and
// kills it if it does not exit.
func (s *server) stop() {
	<-s.ready
	for deadline := time.Now().Add(drainTimeout); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		s.mu.Lock()
		busy := s.inUse > 0
		s.mu.Unlock()
		if !busy {
			break
		}
	}
	if s.err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := s.conn.call(ctx, "shutdown", nil, nil); err == nil {
			_ = s.conn.notify("exit", nil)
		}
		cancel()
		select {
		case <-s.exited:
		case <-time.After(2 * time.Second):
			s.kill()
			<-s.exited
		}
	}
	slog.Debug("language server stopped", "root", s.ws.Root, "language", s.ws.Language)
}

func (s *server) kill() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}
