package settings

import "sync"

// Memory is an in-memory Store for tests.
type Memory struct {
	mu  sync.Mutex
	cfg Settings
}

// NewMemory returns a Memory store holding cfg.
func NewMemory(cfg Settings) *Memory { return &Memory{cfg: cfg} }

func (m *Memory) Load() (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg, nil
}

func (m *Memory) Save(cfg Settings) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	return nil
}
