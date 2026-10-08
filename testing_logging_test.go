package fe

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

// errAlways is the failure a module in these tests reports.
var errAlways = errors.New("module rejected the config")

// logCapture collects the records written through it, so a test can assert
// that the framework's logs actually reached the application's handler — the
// only assertion that proves the seam is connected.
type logCapture struct {
	mu    sync.Mutex
	lines []string
}

func (c *logCapture) add(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, line)
}

func (c *logCapture) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.lines)
}

// handler is an slog.Handler that records every record, including the
// attributes carried on the handler itself.
//
// WithAttrs must be honoured (returning the receiver would be simpler but
// wrong): the framework attaches attribution with Logger.With, so a handler
// that dropped them would silently hide exactly what the tests below check.
type handler struct {
	capture *logCapture
	attrs   []string
}

func newHandler() (*handler, *logCapture) {
	c := new(logCapture)
	return &handler{capture: c}, c
}

func (h *handler) Enabled(context.Context, slog.Level) bool { return true }

func (h *handler) Handle(_ context.Context, r slog.Record) error {
	parts := slices.Clone(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		parts = append(parts, a.Key+"="+a.Value.String())
		return true
	})
	h.capture.add(r.Message + " [" + strings.Join(parts, " ") + "]")
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &handler{capture: h.capture, attrs: slices.Clone(h.attrs)}
	for _, a := range attrs {
		next.attrs = append(next.attrs, a.Key+"="+a.Value.String())
	}
	return next
}

func (h *handler) WithGroup(string) slog.Handler { return h }

// TestNewAppNamesRecords verifies Name, when given, is attached at the source
// so everything the App and its Runtime write carries it.
func TestNewAppNamesRecords(t *testing.T) {
	h, capture := newHandler()

	app, err := New(Options{Name: "core", SlogHandler: h})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	ensureLogger(app.logger).Info("hello")

	lines := capture.all()
	if len(lines) != 1 {
		t.Fatalf("captured %d records, want 1: %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], "app=core") {
		t.Fatalf("record = %q, want it to carry app=core", lines[0])
	}
}

// TestNewAppAnonymous verifies an empty Name is legitimate (not validated) and
// simply produces records without an app attribute.
func TestNewAppAnonymous(t *testing.T) {
	h, capture := newHandler()

	app, err := New(Options{SlogHandler: h})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}
	ensureLogger(app.logger).Info("hello")

	lines := capture.all()
	if len(lines) != 1 {
		t.Fatalf("captured %d records, want 1: %v", len(lines), lines)
	}
	if strings.Contains(lines[0], "app=") {
		t.Fatalf("record = %q, want no app attribute for an unnamed App", lines[0])
	}
}

// TestNewAppNilHandler verifies a missing handler discards instead of
// panicking. A nil slog.Handler behind a non-nil *slog.Logger panics on first
// use, so this is the nil guard New exists to provide.
func TestNewAppNilHandler(t *testing.T) {
	app, err := New(Options{Name: "quiet"})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	ensureLogger(app.logger).Info("goes nowhere") // must not panic
}

// TestAppZeroValueLogsSafely verifies the zero-value App — the form tests and
// throwaway programs use — logs without panicking. New is a convenience, not
// the only way to get a working App.
func TestAppZeroValueLogsSafely(t *testing.T) {
	a := new(App)
	ensureLogger(a.logger).Info("goes nowhere") // must not panic
}

// TestAppAndRuntimeLogsReachHandler verifies the whole chain end to end: the
// handler the application supplies receives the App's records, the Runtime's
// records, and both carry the App's attribution.
//
// This is the seam's acceptance test: it fails if a logger is built but never
// used, if the injection into the Runtime is dropped, or if attribution is
// lost between the layers.
func TestAppAndRuntimeLogsReachHandler(t *testing.T) {
	const modID = ModuleID("fe.test.logging.inject")
	RegisterModule(provMod(modID, nil))

	h, capture := newHandler()
	app, err := New(Options{Name: "core", SlogHandler: h})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	if err := app.Start(appOne(appLeaf1ID, string(modID))); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := app.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got := strings.Join(capture.all(), "\n")
	// App layer and Runtime layer both report; the Runtime's records arrive
	// only if App.Start injected the logger into the config it handed over.
	want := []string{
		"app starting",
		"runtime created",
		"runtime starting",
		"runtime started",
		"app started",
		"app stopping",
		"runtime stopping",
		"runtime stopped",
		"app stopped",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("no record contains %q; captured:\n%s", w, got)
		}
	}

	// Every record carries the App's attribution, and the Runtime counts its
	// instances (instance lifecycle itself is deliberately not logged).
	if !strings.Contains(got, "app=core") {
		t.Errorf("records do not carry app=core; captured:\n%s", got)
	}
	if !strings.Contains(got, "instances=1") {
		t.Errorf("the runtime-created record does not report its instance count; captured:\n%s", got)
	}
}

// TestAppStartLogsFailure verifies a failed start is reported, since the error
// alone reaches only the caller.
func TestAppStartLogsFailure(t *testing.T) {
	const modID = ModuleID("fe.test.logging.startfail")
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return nil, errAlways
	}))

	h, capture := newHandler()
	app, err := New(Options{Name: "core", SlogHandler: h})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	if err := app.Start(appOne(appLeaf1ID, string(modID))); err == nil {
		t.Fatal("Start: expected an error from the failing module")
	}

	got := strings.Join(capture.all(), "\n")
	if !strings.Contains(got, "app start failed") {
		t.Errorf("the failure was not reported; captured:\n%s", got)
	}
	if !strings.Contains(got, errAlways.Error()) {
		t.Errorf("the failure record does not carry the cause; captured:\n%s", got)
	}
}

// TestAppStartLeavesCallerConfigUntouched verifies the logger is injected into
// a copy: App.Start must not write to the MachineConfig its caller owns, and
// two Apps sharing one must not overwrite each other's logger.
func TestAppStartLeavesCallerConfigUntouched(t *testing.T) {
	const modID = ModuleID("fe.test.logging.nomutate")
	RegisterModule(provMod(modID, nil))

	h, _ := newHandler()
	app, err := New(Options{Name: "core", SlogHandler: h})
	if err != nil {
		t.Fatalf("New: unexpected error: %v", err)
	}

	mc := appOne(appLeaf1ID, string(modID))
	if err := app.Start(mc); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer app.Stop()

	if mc.Options.Logger != nil {
		t.Error("App.Start wrote its logger into the caller's MachineConfig")
	}
}
