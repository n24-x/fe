package fe

import (
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// Manager owns the currently active Runtime. Apply swaps in new Runtimes on
// config change; Stop shuts the active one down on process exit.
//
// TODO(next):
// Manager is the seam reserved for future hot reload of the machine config
// (mirroring caddy's currentCtx / changeConfig mechanism). The Runtime flow
// itself (NewRuntime) does not depend on Manager.
type Manager struct {
	mu sync.Mutex
	// current is the active Runtime, if any.
	current *Runtime
}

// Apply replaces the active config: it builds and starts a new Runtime
// from mc and, only on success, swaps it in and stops the old one.
//
// TODO(next):
// This is the future hot-reload entry point (issue.md D5/D7).
// Build-then-swap (caddy's model): NewRuntime provisions every instance and
// Start runs them. If either fails, the new Runtime is discarded — a failed
// Start has already stopped what it started and closed its lifecycle signal —
// and the old Runtime keeps running untouched.
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

// Stop shuts the active Runtime down (reverse start order + lifecycle signal).
// This is the process-exit path: the application calls Stop on SIGINT/SIGTERM
// after having started the config with Apply. It is a no-op when no Runtime
// is active, so it is safe to call unconditionally on shutdown.
//
// Stop is the Manager-side counterpart of Apply: Apply starts a config and
// makes it current; Stop ends the current one. (caddy's Stop is the
// "antithesis of Run" — this is the fe equivalent.)
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil {
		return nil
	}
	err := m.current.Stop()
	m.current = nil
	return err
}
