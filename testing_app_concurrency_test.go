package fe

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/n24-x/fe/feconfig"
)

// The App's concurrency contract, in testable pieces:
//
//   - Start runs module code (Provision, then Start/Stop) outside its state
//     lock, so Stop is not made to wait for a reload.
//   - Stop is terminal, so a reload already in flight when it happens cannot
//     install the Runtime it built.
//   - The Runtime is taken out of the App under the lock before it is
//     stopped, so concurrent Stops cannot both reach Runtime.Stop (which
//     cannot be called twice concurrently).

// gateInstance blocks in Start until its release channel is closed, which is
// how these tests hold a Start open in the middle of module code.
type gateInstance struct {
	name    string
	trace   *lifecycleTrace
	entered chan struct{}
	release chan struct{}
}

func (g *gateInstance) Start() error {
	g.trace.add("start:" + g.name)
	close(g.entered)
	<-g.release
	g.trace.add("started:" + g.name)
	return nil
}

func (g *gateInstance) Stop() error {
	g.trace.add("stop:" + g.name)
	return nil
}

// registerGateMod registers a single-instance module whose instance blocks in
// Start. It returns the gate and a release func, also registered with
// t.Cleanup so a failing test cannot leave a goroutine stuck; release is
// idempotent.
func registerGateMod(t *testing.T, modID ModuleID, trace *lifecycleTrace) (*gateInstance, func()) {
	t.Helper()
	gate := &gateInstance{
		name:    "g",
		trace:   trace,
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return gate, nil
	}))

	released := false
	release := func() {
		if !released {
			released = true
			close(gate.release)
		}
	}
	t.Cleanup(release)
	return gate, release
}

// appOne returns a one-instance config, for tests that do not need a chain.
func appOne(id, modID string) *feconfig.MachineConfig {
	return &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{{InstanceID: id, ModuleID: modID}},
	}
}

// TestAppStopDoesNotWaitForStart verifies Stop returns promptly while a Start
// is blocked inside module code.
//
// With the state lock held across NewRuntime/Start (the earlier design), Stop
// blocked until the whole reload finished — so a SIGINT arriving during a slow
// reload hung the shutdown for as long as that reload took.
func TestAppStopDoesNotWaitForStart(t *testing.T) {
	const modID = ModuleID("fe.test.app.nonblocking")
	trace := new(lifecycleTrace)
	gate, release := registerGateMod(t, modID, trace)

	a := new(App)
	startDone := make(chan error, 1)
	go func() { startDone <- a.Start(appOne(appLeaf1ID, string(modID))) }()

	// Wait until Start is inside instance code, i.e. inside module code.
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Start never reached the instance")
	}

	// Stop must not wait for that Start to return.
	stopDone := make(chan error, 1)
	go func() { stopDone <- a.Stop() }()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop blocked while Start was inside module code")
	}

	release() // let the in-flight Start finish

	// Stop was terminal, so the Runtime Start built was not installed; it was
	// stopped instead, and Start says why.
	if err := <-startDone; !errors.Is(err, ErrAppStopped) {
		t.Fatalf("Start error = %v, want errors.Is(err, ErrAppStopped)", err)
	}
	if a.current != nil {
		t.Fatal("a stopped App must not hold a Runtime")
	}
	if want := "start:g started:g stop:g"; trace.got() != want {
		t.Fatalf("trace = %q, want %q (the in-flight Runtime must be stopped, not leaked)", trace.got(), want)
	}
}

// TestAppStartAfterStop verifies Stop is terminal: a later Start fails with
// ErrAppStopped instead of quietly starting a Runtime that nothing will ever
// stop.
func TestAppStartAfterStop(t *testing.T) {
	const modID = ModuleID("fe.test.app.afterstop")
	trace := new(lifecycleTrace)
	registerAppMod(t, modID, map[string]string{appLeaf1ID: "a"}, trace, nil, nil)

	a := new(App)
	if err := a.Start(appOne(appLeaf1ID, string(modID))); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	err := a.Start(appOne(appLeaf1ID, string(modID)))
	if !errors.Is(err, ErrAppStopped) {
		t.Fatalf("Start after Stop = %v, want errors.Is(err, ErrAppStopped)", err)
	}
	// The Runtime it built was started and then stopped again: nothing left
	// running, nothing left installed.
	if want := "start:a stop:a start:a stop:a"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
	if a.current != nil {
		t.Fatal("Start after Stop must not leave a Runtime installed")
	}
}

// TestAppConcurrentStops verifies concurrent Stop calls cannot both reach
// Runtime.Stop: the first takes the active Runtime out from under the lock, so
// the rest find nothing to do.
func TestAppConcurrentStops(t *testing.T) {
	const modID = ModuleID("fe.test.app.cstop")
	trace := new(lifecycleTrace)
	registerAppMod(t, modID, map[string]string{appLeaf1ID: "a"}, trace, nil, nil)

	a := new(App)
	if err := a.Start(appOne(appLeaf1ID, string(modID))); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const stops = 8
	done := make(chan error, stops)
	for range stops {
		go func() { done <- a.Stop() }()
	}
	for range stops {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Stop: %v", err)
		}
	}
	if want := "start:a stop:a"; trace.got() != want {
		t.Fatalf("trace = %q, want %q (the instance must be stopped exactly once)", trace.got(), want)
	}
}

// TestAppConcurrentStarts verifies Start is serialized: overlapping reloads
// must not interleave their build/swap/stop sequences.
func TestAppConcurrentStarts(t *testing.T) {
	const modID = ModuleID("fe.test.app.cstart")
	trace := new(lifecycleTrace)
	registerAppMod(t, modID, map[string]string{appLeaf1ID: "a"}, trace, nil, nil)

	a := new(App)
	const n = 8
	done := make(chan error, n)
	for range n {
		go func() { done <- a.Start(appOne(appLeaf1ID, string(modID))) }()
	}
	for range n {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Start: %v", err)
		}
	}
	if a.current == nil {
		t.Fatal("after concurrent Starts, a Runtime must be current")
	}

	// Each Start but the first also stops the Runtime it replaced, and the
	// final Stop ends the last one — so n Starts produce exactly n instance
	// starts and n stops. Interleaving would strand or double-stop a
	// generation.
	if err := a.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	ev := trace.events
	startCount := strings.Count(strings.Join(ev, " "), "start:")
	if startCount != n {
		t.Fatalf("starts = %d, want %d (trace = %q)", startCount, n, trace.got())
	}
	if stopCount := len(ev) - startCount; stopCount != n {
		t.Fatalf("stops = %d, want %d (trace = %q)", stopCount, n, trace.got())
	}
}
