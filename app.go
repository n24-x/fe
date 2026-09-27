package fe

import (
	"log/slog"
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// App is the framework user's handle on the active Runtime — to a Runtime
// what a [Module] is to an Instance. Start installs one (hot-replacing the
// previous), Stop ends it.
type App struct {
	// loadMu serializes Runtime loads. [App.Start] holds it, so only one Runtime
	// can be built and installed at a time. [App.Stop] is deliberately
	// outside it.
	loadMu sync.Mutex

	// mu guards the fields: current and stopped. Never held across module code.
	mu sync.Mutex
	// current is the active Runtime, if any.
	current *Runtime
	// stopped records that the App has been stopped.
	// Once stopped, it cannot be started again.
	stopped bool

	// name labels this App.
	name string

	// logger is the App's logger, provided by the application. It is used to
	// create the logger for each Runtime.
	logger *slog.Logger
}

// Options configures an [App] in [New]. Every member is optional and none is
// validated.
type Options struct {
	// Name labels the App, as the "app" attribute, in the records it produces —
	// and, by derivation, in its Runtime's. Empty means no attribute.
	Name string

	// SlogHandler receives every record the framework reports about this [App],
	// its Runtimes, and (through them) their instances.
	//
	// There is no default: nil discards. See
	// https://github.com/n24-x/fe/blob/main/docs/logging.md for what to pass and
	// why it is a [slog.Handler], not a logger.
	SlogHandler slog.Handler
}

// New returns an [App].
//
// The error is always nil today — an App never fails to construct — and is in
// the signature so a future check (a reserved name, say) does not break
// callers.
func New(opts Options) (*App, error) {
	logger := loggerFrom(opts.SlogHandler)
	if opts.Name != "" {
		// Attach the name at the source so every derived logger — the Runtime's
		// and every logger below it — carries it without repeating the call.
		logger = logger.With("app", opts.Name)
	}
	return &App{name: opts.Name, logger: logger}, nil
}

// Start builds and starts a new Runtime from mc. On success, it swaps in the
// new [Runtime] and stops the old Runtime. If building or starting the new Runtime
// fails, Start returns the error without changing the current Runtime.
//
// If [App.Stop] runs while the new Runtime is being built, Start stops the
// Runtime it built and returns [ErrAppStopped].
//
// Start can be used to reload the App with a new config.
func (a *App) Start(mc *feconfig.MachineConfig) error {
	a.loadMu.Lock()
	defer a.loadMu.Unlock()

	log := ensureLogger(a.logger)
	log.Info("app starting")

	// Work on a copy so the caller's MachineConfig is never written to: two
	// Apps sharing one would otherwise race on the Logger field set below.
	mcCopy := *mc
	mcCopy.Options.Logger = log // inject Logger

	// Build outside mu: module code may block, and MUST NOT block [App.Stop].
	r, err := NewRuntime(&mcCopy)
	if err != nil {
		log.Error("app start failed", "err", err)
		return err
	}
	if err := r.Start(); err != nil {
		log.Error("app start failed", "err", err)
		return err
	}

	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		_ = r.Stop()
		log.Info("app start/reload abandoned: app stopped")
		return ErrAppStopped
	}
	old := a.current
	a.current = r
	a.mu.Unlock()

	if old != nil {
		log.Info("app replacing runtime")
		old.Stop() // best-effort: the old runtime is being replaced regardless
	}
	log.Info("app started")
	return nil
}

// Stop stops the App and its active Runtime. It is a no-op when no Runtime is
// active, so it is safe to call unconditionally on the shutdown path.
//
// Stop is final: a stopped App cannot be started again. A later [App.Start]
// fails with [ErrAppStopped]. The App itself has no resources to clean up; any
// returned error comes from stopping the Runtime. The App is stopped regardless
// of any error, so a second Stop call returns nil.
func (a *App) Stop() error {
	// Take the Runtime out under mu, then stop it outside: Runtime.Stop runs
	// module code and may take a long time. Clearing current first also
	// guarantees a single caller reaches [Runtime.Stop] — it is idempotent
	// sequentially but not safe to enter concurrently.
	a.mu.Lock()
	r := a.current
	a.current = nil
	a.stopped = true
	a.mu.Unlock()

	if r == nil {
		return nil
	}

	log := ensureLogger(a.logger)
	log.Info("app stopping")
	err := r.Stop()
	if err != nil {
		log.Error("app stop failed", "err", err)
		return err
	}
	log.Info("app stopped")
	return nil
}
