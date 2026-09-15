package fe

import (
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// App is a framework user's handle on one config slot: it owns the Runtime
// built from that config, Start builds and starts a replacement on config
// change, and Stop ends the active one (the process-exit path).
//
// The two kinds of fe user each get one word: a module developer implements
// [Module] (and [Provisioner]) and has instances loaded into a config; a
// framework user drives an App. An App is to a Runtime what a Module is to an
// Instance — the longer-lived object that produces and owns the shorter-lived
// one.
//
// # Concurrency contract
//
// Two locks, because the work Start does is user code (Provision, then every
// instance's Start and Stop) and may take arbitrarily long:
//
//   - startMu serializes Start, so two reloads cannot interleave.
//   - mu guards current and stopped only. It is never held across user code,
//     so Stop can preempt an in-flight Start rather than waiting for it — the
//     difference between a SIGINT being honored promptly and the process
//     hanging for as long as a reload takes.
//
// TODO(next):
// App is the seam reserved for future hot reload of the machine config
// (mirroring caddy's currentCtx / changeConfig mechanism). The Runtime flow
// itself (NewRuntime) does not depend on App.
type App struct {
	// startMu serializes Start: one reload at a time.
	startMu sync.Mutex

	// mu guards the fields below. Never held across user code.
	mu sync.Mutex
	// current is the active Runtime, if any.
	current *Runtime
	// stopped records that Stop ran. Stop is terminal, so the App is
	// single-use: a Start already in flight when it happens must not install
	// the Runtime it just built (see Start).
	stopped bool
}

// Start makes mc the active config: it builds and starts a new Runtime from
// mc and, only on success, swaps it in and stops the one it replaced.
//
// It returns ErrAppStopped, having stopped the Runtime it built, if Stop ran
// while that Runtime was being built — a shutdown must not be outrun by a
// reload that was already under way.
//
// TODO(next):
// This is the future hot-reload entry point (issue.md D5/D7).
// Build-then-swap (caddy's model): NewRuntime provisions every instance and
// Start runs them. If either fails, the new Runtime is discarded — a failed
// Start has already stopped what it started and closed its lifecycle signal —
// and the old Runtime keeps running untouched.
func (a *App) Start(mc *feconfig.MachineConfig) error {
	a.startMu.Lock()
	defer a.startMu.Unlock()

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

	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		// Stop ran while this Runtime was being built, so it never became
		// current and nothing else will ever stop it. Install nothing, and
		// release it here rather than leak a running Runtime.
		_ = r.Stop()
		return ErrAppStopped
	}
	old := a.current
	a.current = r
	a.mu.Unlock()

	if old != nil {
		old.Stop() // best-effort: the old tree is being replaced regardless
	}
	return nil
}

// Stop shuts the active Runtime down (reverse start order + lifecycle signal).
// This is the process-exit path: the application calls Stop on SIGINT/SIGTERM
// after having started a config with Start. It is a no-op when no Runtime is
// active, so it is safe to call unconditionally on shutdown.
//
// Stop is terminal: the App is single-use, and a later Start fails with
// ErrAppStopped. That is what makes the shutdown authoritative — Stop cannot
// be outrun by a reload that is already building a Runtime.
//
// Stop is the App-side counterpart of Start: Start makes a config current;
// Stop ends the current one. (caddy's Stop is the "antithesis of Run" — this
// is the fe equivalent.)
func (a *App) Stop() error {
	// Take the active Runtime out of the App under mu, then stop it outside:
	// instance Stop is user code too, and holding mu across it would block
	// Start for as long as teardown takes. Clearing it first also means
	// concurrent Stop calls cannot both reach Runtime.Stop — only one of them
	// gets a non-nil Runtime to stop. (Runtime.Stop is not safe to call twice
	// concurrently: the second close of the lifecycle channel panics.)
	a.mu.Lock()
	r := a.current
	a.current = nil
	a.stopped = true
	a.mu.Unlock()

	if r == nil {
		return nil
	}
	return r.Stop()
}
