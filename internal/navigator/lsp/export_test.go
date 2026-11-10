package lsp

import "github.com/Coombszy/scrutiny/internal/navigator"

// PID returns the process ID of ws's server, or 0.
func (m *Manager) PID(ws navigator.Workspace) int {
	m.mu.Lock()
	s, ok := m.servers[ws]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	<-s.ready
	if s.cmd == nil || s.cmd.Process == nil {
		return 0
	}
	return s.cmd.Process.Pid
}
