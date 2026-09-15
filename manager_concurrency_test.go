package fe

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/n24-x/fe/feconfig"
)

// The Manager's concurrency contract, in testable pieces:
//
//   - Apply runs module code (Provision, then Start/Stop) outside its state
//     lock, so Stop is not made to wait for a reload.
//   - Stop is terminal, so a reload already in flight when it happens cannot
//     install the Runtime it built.
//   - The Runtime is taken out of the Manager under the lock before it is
//     stopped, so concurrent Stops cannot both reach Runtime.Stop (which
//     cannot be called twice concurrently).

// gateInstance blocks in Start until its release channel is closed, which is
// how these tests hold an Apply open in the middle of module code.
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

// mgrOne returns a one-instance config, for tests that do not need a chain.
func mgrOne(id, modID string) *feconfig.MachineConfig {
	return &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{{InstanceID: id, ModuleID: modID}},
	}
}

// TestManagerStopDoesNotWaitForApply verifies Stop returns promptly while an
// Apply is blocked inside module code.
//
// With the state lock held across NewRuntime/Start (the earlier design), Stop
// blocked until the whole reload finished — so a SIGINT arriving during a slow
// reload hung the shutdown for as long as that reload took.
func TestManagerStopDoesNotWaitForApply(t *testing.T) {
	const modID = ModuleID("fe.test.manager.nonblocking")
	trace := new(lifecycleTrace)
	gate, release := registerGateMod(t, modID, trace)

	m := new(Manager)
	applyDone := make(chan error, 1)
	go func() { applyDone <- m.Apply(mgrOne(mgrLeaf1ID, string(modID))) }()

	// Wait until Apply is inside Start, i.e. inside module code.
	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("Apply never reached Start")
	}

	// Stop must not wait for that Start to return.
	stopDone := make(chan error, 1)
	go func() { stopDone <- m.Stop() }()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop blocked while Apply was inside module code")
	}

	release() // let the in-flight Apply finish

	// Stop was terminal, so the Runtime Apply built was not installed; it was
	// stopped instead, and Apply says why.
	if err := <-applyDone; !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("Apply error = %v, want errors.Is(err, ErrManagerStopped)", err)
	}
	if m.current != nil {
		t.Fatal("a stopped Manager must not hold a Runtime")
	}
	if want := "start:g started:g stop:g"; trace.got() != want {
		t.Fatalf("trace = %q, want %q (the in-flight Runtime must be stopped, not leaked)", trace.got(), want)
	}
}

// TestManagerApplyAfterStop verifies Stop is terminal: a later Apply fails
// with ErrManagerStopped instead of quietly starting a Runtime that nothing
// will ever stop.
func TestManagerApplyAfterStop(t *testing.T) {
	const modID = ModuleID("fe.test.manager.afterstop")
	trace := new(lifecycleTrace)
	registerMgrMod(t, modID, map[string]string{mgrLeaf1ID: "a"}, trace, nil, nil)

	m := new(Manager)
	if err := m.Apply(mgrOne(mgrLeaf1ID, string(modID))); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	err := m.Apply(mgrOne(mgrLeaf1ID, string(modID)))
	if !errors.Is(err, ErrManagerStopped) {
		t.Fatalf("Apply after Stop = %v, want errors.Is(err, ErrManagerStopped)", err)
	}
	// The Runtime it built was started and then stopped again: nothing left
	// running, nothing left installed.
	if want := "start:a stop:a start:a stop:a"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
	if m.current != nil {
		t.Fatal("Apply after Stop must not leave a Runtime installed")
	}
}

// TestManagerConcurrentStops verifies concurrent Stop calls cannot both reach
// Runtime.Stop: the first takes the active Runtime out from under the lock, so
// the rest find nothing to do.
func TestManagerConcurrentStops(t *testing.T) {
	const modID = ModuleID("fe.test.manager.cstop")
	trace := new(lifecycleTrace)
	registerMgrMod(t, modID, map[string]string{mgrLeaf1ID: "a"}, trace, nil, nil)

	m := new(Manager)
	if err := m.Apply(mgrOne(mgrLeaf1ID, string(modID))); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	const stops = 8
	done := make(chan error, stops)
	for range stops {
		go func() { done <- m.Stop() }()
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

// TestManagerConcurrentApplies verifies Apply is serialized: overlapping
// reloads must not interleave their build/swap/stop sequences.
func TestManagerConcurrentApplies(t *testing.T) {
	const modID = ModuleID("fe.test.manager.capply")
	trace := new(lifecycleTrace)
	registerMgrMod(t, modID, map[string]string{mgrLeaf1ID: "a"}, trace, nil, nil)

	m := new(Manager)
	const applies = 8
	done := make(chan error, applies)
	for range applies {
		go func() { done <- m.Apply(mgrOne(mgrLeaf1ID, string(modID))) }()
	}
	for range applies {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Apply: %v", err)
		}
	}
	if m.current == nil {
		t.Fatal("after concurrent Applies, a Runtime must be current")
	}

	// Each Apply but the first also stops the Runtime it replaced, and the
	// final Stop ends the last one — so n Applies produce exactly n starts and
	// n stops. Interleaving would strand or double-stop a generation.
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	ev := trace.events
	starts := strings.Count(strings.Join(ev, " "), "start:")
	if starts != applies {
		t.Fatalf("starts = %d, want %d (trace = %q)", starts, applies, trace.got())
	}
	if got := len(ev) - starts; got != applies {
		t.Fatalf("stops = %d, want %d (trace = %q)", got, applies, trace.got())
	}
}
