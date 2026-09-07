package fe

import (
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// Manager is the process-level owner of the currently active Runtime.
//
// Its only role today is architectural: it is the seam reserved for future
// hot reload of the machine config (mirroring caddy's currentCtx /
// changeConfig mechanism). The Runtime flow itself (NewRuntime) does not
// depend on Manager. Nothing calls Manager yet.
type Manager struct {
	mu sync.Mutex
	// current is the active Runtime, if any.
	current *Runtime
}

// Apply replaces the active config: it builds and starts a brand-new Runtime
// from mc and, only on success, swaps it in and stops the old one.
// This is the future hot-reload entry point (issue.md D5/D7).
//
// Build-then-swap (caddy's model): NewRuntime provisions every instance and
// Start runs them. If either fails, the new Runtime is discarded — a failed
// Start has already stopped what it started and canceled its context — and
// the old Runtime keeps running untouched.
func (m *Manager) Apply(mc *feconfig.MachineConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, err := NewRuntime(mc)
	if err != nil {
		return err
	}
	if err := r.Start(); err != nil {
		return err
	}

	old := m.current
	m.current = r
	if old != nil {
		old.Stop() // best-effort: the old tree is being replaced regardless
	}
	return nil
}
