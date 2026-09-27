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

	// mu guards the fields: current and stopped. Never held across user code.
	mu sync.Mutex
	// current is the active Runtime, if any.
	current *Runtime
	// stopped records that the App has been stopped.
	// Once stopped, it cannot be started again.
	stopped bool

	// name labels this App.
	name string

	// logger records this App's and its Runtime's lifecycle. If none, it discards,
	// and the App stays silent. See logging.go for the seam.
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
	// There is no default: nil discards. See logging.go for what to pass
	// and why it is a [slog.Handler], not a logger.
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

	log := orDiscard(a.logger)
	log.Info("app starting")

	// Runtime.cfg is read-only, and two Apps sharing one *MachineConfig
	// would otherwise overwrite some fields.
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

	log := orDiscard(a.logger)
	log.Info("app stopping")
	err := r.Stop()
	if err != nil {
		log.Error("app stop failed", "err", err)
		return err
	}
	log.Info("app stopped")
	return nil
}
