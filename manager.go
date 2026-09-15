package fe

import (
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// Manager owns the currently active Runtime. Apply swaps in new Runtimes on
// config change; Stop shuts the active one down on process exit.
//
// # Concurrency contract
//
// Two locks, because the work Apply does is user code (Provision, then every
// instance's Start and Stop) and may take arbitrarily long:
//
//   - applyMu serializes Apply, so two reloads cannot interleave.
//   - mu guards current and stopped only. It is never held across user code,
//     so Stop can preempt an in-flight Apply instead of waiting for it — the
//     difference between a SIGINT being honored promptly and the process
//     hanging for as long as a reload takes.
//
// TODO(next):
// Manager is the seam reserved for future hot reload of the machine config
// (mirroring caddy's currentCtx / changeConfig mechanism). The Runtime flow
// itself (NewRuntime) does not depend on Manager.
type Manager struct {
	// applyMu serializes Apply: one reload at a time.
	applyMu sync.Mutex

	// mu guards the fields below. Never held across user code.
	mu sync.Mutex
	// current is the active Runtime, if any.
	current *Runtime
	// stopped records that Stop ran. Stop is terminal, so the Manager is
	// single-use: an Apply already in flight when it happens must not install
	// the Runtime it just started (see Apply).
	stopped bool
}

// Apply replaces the active config: it builds and starts a new Runtime
// from mc and, only on success, swaps it in and stops the old one.
//
// It returns ErrManagerStopped, having stopped the Runtime it built, if Stop
// ran while that Runtime was being built — a shutdown must not be outrun by a
// reload that was already under way.
//
// TODO(next):
// This is the future hot-reload entry point (issue.md D5/D7).
// Build-then-swap (caddy's model): NewRuntime provisions every instance and
// Start runs them. If either fails, the new Runtime is discarded — a failed
// Start has already stopped what it started and closed its lifecycle signal —
// and the old Runtime keeps running untouched.
func (m *Manager) Apply(mc *feconfig.MachineConfig) error {
	m.applyMu.Lock()
	defer m.applyMu.Unlock()

	// Build and start outside mu: this is module code that may block on a
	// network bind, a slow decode, anything. Holding mu here is what used to
	// make a concurrent Stop wait for the whole reload.
	r, err := NewRuntime(mc)
	if err != nil {
		return err
	}
	if err := r.Start(); err != nil {
		return err
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		// Stop ran while this Runtime was being built, so it never became
		// current and nothing else will ever stop it. Install nothing, and
		// release it here rather than leak a running Runtime.
		_ = r.Stop()
		return ErrManagerStopped
	}
	old := m.current
	m.current = r
	m.mu.Unlock()

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
// Stop is terminal: the Manager is single-use, and a later Apply fails with
// ErrManagerStopped. That is what makes the shutdown authoritative — Stop
// cannot be outrun by a reload that is already building a Runtime.
//
// Stop is the Manager-side counterpart of Apply: Apply starts a config and
// makes it current; Stop ends the current one. (caddy's Stop is the
// "antithesis of Run" — this is the fe equivalent.)
func (m *Manager) Stop() error {
	// Take the active Runtime out of the Manager under mu, then stop it
	// outside: instance Stop is user code too, and holding mu across it would
	// block Apply for as long as teardown takes. Clearing it first also means
	// concurrent Stop calls cannot both reach Runtime.Stop — only one of them
	// gets a non-nil Runtime to stop. (Runtime.Stop is not safe to call twice
	// concurrently: the second close of the lifecycle channel panics.)
	m.mu.Lock()
	r := m.current
	m.current = nil
	m.stopped = true
	m.mu.Unlock()

	if r == nil {
		return nil
	}
	return r.Stop()
}
