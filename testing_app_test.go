package fe

import (
	"errors"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

// appInstance is an Instance that records Start/Stop into a shared trace.
// failStart/failStop simulate instance failures; panicStart simulates a module
// bug that panics instead of returning (per-module: every instance of a failing
// module carries them).
type appInstance struct {
	name       string
	trace      *lifecycleTrace
	failStart  error
	failStop   error
	panicStart bool
}

func (i *appInstance) Start() error {
	if i.panicStart {
		panic("appInstance: Start panic")
	}
	if i.failStart != nil {
		return i.failStart
	}
	i.trace.add("start:" + i.name)
	return nil
}

func (i *appInstance) Stop() error {
	if i.failStop != nil {
		return i.failStop
	}
	i.trace.add("stop:" + i.name)
	return nil
}

// Instance ids for the two config generations used by the App tests.
// Tree 1: leaf1 → top1. Tree 2: leaf2 → top2.
const (
	appLeaf1ID = "aa2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	appTop1ID  = "bb2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	appLeaf2ID = "cc2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	appTop2ID  = "dd2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
)

// appChain returns a two-instance dep chain (leaf depends on nothing, top
// depends on leaf) for the given ids.
func appChain(leafID, topID, modID string) *feconfig.MachineConfig {
	return &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: topID, ModuleID: modID, Deps: []string{leafID}},
			{InstanceID: leafID, ModuleID: modID},
		},
	}
}

// registerAppMod registers a module provisioning appInstances named by id.
// The whole module can be made to fail on Start or Stop (failStart/failStop).
func registerAppMod(t *testing.T, modID ModuleID, names map[string]string, trace *lifecycleTrace, failStart, failStop error) {
	t.Helper()
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return &appInstance{name: names[spec.InstanceID], trace: trace, failStart: failStart, failStop: failStop}, nil
	}))
}

// TestAppStartFirstAndSwap verifies Start runs the new Runtime and, on a
// second Start, swaps it in and stops the one it replaced in reverse order.
func TestAppStartFirstAndSwap(t *testing.T) {
	const mod1 = ModuleID("fe.test.app.swap.t1")
	const mod2 = ModuleID("fe.test.app.swap.t2")
	trace := new(lifecycleTrace)
	registerAppMod(t, mod1, map[string]string{appLeaf1ID: "a1", appTop1ID: "b1"}, trace, nil, nil)
	registerAppMod(t, mod2, map[string]string{appLeaf2ID: "c1", appTop2ID: "d1"}, trace, nil, nil)

	a := new(App)
	if err := a.Start(appChain(appLeaf1ID, appTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if a.current == nil {
		t.Fatal("first Start: App.current must be set")
	}
	if want := "start:a1 start:b1"; trace.got() != want {
		t.Fatalf("after first Start, trace = %q, want %q", trace.got(), want)
	}

	if err := a.Start(appChain(appLeaf2ID, appTop2ID, string(mod2))); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	// New tree starts first, then the old tree stops in reverse order.
	if want := "start:a1 start:b1 start:c1 start:d1 stop:b1 stop:a1"; trace.got() != want {
		t.Fatalf("after second Start, trace = %q, want %q", trace.got(), want)
	}
}

// TestAppStartFailureKeepsOld verifies a Start whose new Runtime fails to
// start leaves the old Runtime running and untouched.
func TestAppStartFailureKeepsOld(t *testing.T) {
	const mod1 = ModuleID("fe.test.app.startfail.t1")
	const modFail = ModuleID("fe.test.app.startfail.t2")
	boom := errors.New("start boom")
	trace := new(lifecycleTrace)
	registerAppMod(t, mod1, map[string]string{appLeaf1ID: "a1", appTop1ID: "b1"}, trace, nil, nil)
	// Only the chain top (d1) fails to start, so the leaf (c1) starts first
	// and must be rolled back.
	RegisterModule(provMod(modFail, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		inst := &appInstance{name: map[string]string{appLeaf2ID: "c1", appTop2ID: "d1"}[spec.InstanceID], trace: trace}
		if spec.InstanceID == appTop2ID {
			inst.failStart = boom
		}
		return inst, nil
	}))

	a := new(App)
	if err := a.Start(appChain(appLeaf1ID, appTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	old := a.current

	err := a.Start(appChain(appLeaf2ID, appTop2ID, string(modFail)))
	if err == nil {
		t.Fatal("Start: expected error from failing new tree")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Start error = %v, want errors.Is(err, boom)", err)
	}
	if a.current != old {
		t.Fatal("Start failure must not swap App.current")
	}
	// New tree's leaf started, then the App's teardown stopped it; the old tree
	// was never stopped.
	if want := "start:a1 start:b1 start:c1 stop:c1"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
}

// TestAppStartPanicStopsWhatItStarted verifies the teardown runs while a module
// panic unwinds: the instance that started before the panic is stopped, and the
// Runtime is not installed.
func TestAppStartPanicStopsWhatItStarted(t *testing.T) {
	const modPanic = ModuleID("fe.test.app.panic.t1")
	trace := new(lifecycleTrace)
	// The chain top panics; the leaf starts first and must be stopped again.
	RegisterModule(provMod(modPanic, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		inst := &appInstance{name: map[string]string{appLeaf2ID: "c1", appTop2ID: "d1"}[spec.InstanceID], trace: trace}
		if spec.InstanceID == appTop2ID {
			inst.panicStart = true
		}
		return inst, nil
	}))

	a := new(App)
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Start: expected the panic to reach the caller")
			}
		}()
		_ = a.Start(appChain(appLeaf2ID, appTop2ID, string(modPanic)))
	}()

	if a.current != nil {
		t.Fatal("a panicking Start must not install a Runtime")
	}
	if want := "start:c1 stop:c1"; trace.got() != want {
		t.Fatalf("trace = %q, want %q (the teardown must run while unwinding)", trace.got(), want)
	}
}

// TestAppStartBuildFailureKeepsOld verifies a Start whose new Runtime fails
// during construction (Provision) leaves the old one untouched.
func TestAppStartBuildFailureKeepsOld(t *testing.T) {
	const mod1 = ModuleID("fe.test.app.buildfail.t1")
	const modBad = ModuleID("fe.test.app.buildfail.t2")
	boom := errors.New("provision boom")
	trace := new(lifecycleTrace)
	registerAppMod(t, mod1, map[string]string{appLeaf1ID: "a1", appTop1ID: "b1"}, trace, nil, nil)
	RegisterModule(provMod(modBad, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return nil, boom
	}))

	a := new(App)
	if err := a.Start(appChain(appLeaf1ID, appTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	old := a.current

	err := a.Start(appChain(appLeaf2ID, appTop2ID, string(modBad)))
	if err == nil {
		t.Fatal("Start: expected error from failing provision")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Start error = %v, want errors.Is(err, boom)", err)
	}
	if a.current != old {
		t.Fatal("Start failure must not swap App.current")
	}
	if want := "start:a1 start:b1"; trace.got() != want {
		t.Fatalf("old tree was disturbed: trace = %q, want %q", trace.got(), want)
	}
}

// TestAppStartOldStopErrorIgnored verifies a Stop error from the tree being
// replaced is best-effort and does not fail Start.
func TestAppStartOldStopErrorIgnored(t *testing.T) {
	const mod1 = ModuleID("fe.test.app.stoperr.t1")
	const mod2 = ModuleID("fe.test.app.stoperr.t2")
	stopBoom := errors.New("old stop boom")
	trace := new(lifecycleTrace)
	registerAppMod(t, mod1, map[string]string{appLeaf1ID: "a1", appTop1ID: "b1"}, trace, nil, stopBoom)
	registerAppMod(t, mod2, map[string]string{appLeaf2ID: "c1", appTop2ID: "d1"}, trace, nil, nil)

	a := new(App)
	if err := a.Start(appChain(appLeaf1ID, appTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Start: %v", err)
	}

	// The old tree's Stop fails, but Start must still succeed (swap completed).
	if err := a.Start(appChain(appLeaf2ID, appTop2ID, string(mod2))); err != nil {
		t.Fatalf("second Start: expected nil despite old tree Stop failure, got %v", err)
	}
	if a.current == nil {
		t.Fatal("App.current must hold the new Runtime")
	}
	// Old tree's Stop failed before recording (failStop short-circuits), so
	// only the starts appear.
	if want := "start:a1 start:b1 start:c1 start:d1"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
}

// TestAppStop verifies Stop shuts the active Runtime down in reverse order,
// clears it, and is a no-op once nothing is active.
func TestAppStop(t *testing.T) {
	const modID = ModuleID("fe.test.app.stop")
	trace := new(lifecycleTrace)
	registerAppMod(t, modID, map[string]string{appLeaf1ID: "a1", appTop1ID: "b1"}, trace, nil, nil)

	a := new(App)
	if err := a.Start(appChain(appLeaf1ID, appTop1ID, string(modID))); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if want := "start:a1 start:b1"; trace.got() != want {
		t.Fatalf("after Start, trace = %q, want %q", trace.got(), want)
	}

	if err := a.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if want := "start:a1 start:b1 stop:b1 stop:a1"; trace.got() != want {
		t.Fatalf("after Stop, trace = %q, want %q", trace.got(), want)
	}
	if a.current != nil {
		t.Fatal("after Stop, App.current must be cleared")
	}

	// No active Runtime → second Stop is a no-op, not an error.
	if err := a.Stop(); err != nil {
		t.Fatalf("second Stop: expected nil (no active Runtime), got %v", err)
	}
	if want := "start:a1 start:b1 stop:b1 stop:a1"; trace.got() != want {
		t.Fatalf("second Stop changed trace: %q, want %q", trace.got(), want)
	}
}
