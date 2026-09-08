package fe

import (
	"errors"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

// mgrInstance is an Instance that records Start/Stop into a shared trace.
// failStart/failStop simulate instance failures (per-module: every instance
// of a failing module carries them).
type mgrInstance struct {
	name      string
	trace     *lifecycleTrace
	failStart error
	failStop  error
}

func (i *mgrInstance) Start() error {
	if i.failStart != nil {
		return i.failStart
	}
	i.trace.add("start:" + i.name)
	return nil
}

func (i *mgrInstance) Stop() error {
	if i.failStop != nil {
		return i.failStop
	}
	i.trace.add("stop:" + i.name)
	return nil
}

// Instance ids for the two config generations used by the Manager tests.
// Tree 1: leaf1 → top1. Tree 2: leaf2 → top2.
const (
	mgrLeaf1ID = "aa2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	mgrTop1ID  = "bb2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	mgrLeaf2ID = "cc2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	mgrTop2ID  = "dd2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
)

// mgrChain returns a two-instance dep chain (leaf depends on nothing, top
// depends on leaf) for the given ids.
func mgrChain(leafID, topID, modID string) *feconfig.MachineConfig {
	return &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: topID, ModuleID: modID, Deps: []string{leafID}},
			{InstanceID: leafID, ModuleID: modID},
		},
	}
}

// registerMgrMod registers a module provisioning mgrInstances named by id.
// The whole module can be made to fail on Start or Stop (failStart/failStop).
func registerMgrMod(t *testing.T, modID ModuleID, names map[string]string, trace *lifecycleTrace, failStart, failStop error) {
	t.Helper()
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return &mgrInstance{name: names[spec.InstanceID], trace: trace, failStart: failStart, failStop: failStop}, nil
	}))
}

// TestManagerApplyFirstAndSwap verifies Apply starts the new Runtime and, on
// a subsequent Apply, swaps it in and stops the old one in reverse order.
func TestManagerApplyFirstAndSwap(t *testing.T) {
	const mod1 = ModuleID("fe.test.manager.swap.t1")
	const mod2 = ModuleID("fe.test.manager.swap.t2")
	trace := new(lifecycleTrace)
	registerMgrMod(t, mod1, map[string]string{mgrLeaf1ID: "a1", mgrTop1ID: "b1"}, trace, nil, nil)
	registerMgrMod(t, mod2, map[string]string{mgrLeaf2ID: "c1", mgrTop2ID: "d1"}, trace, nil, nil)

	m := new(Manager)
	if err := m.Apply(mgrChain(mgrLeaf1ID, mgrTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	if m.current == nil {
		t.Fatal("first Apply: Manager.current must be set")
	}
	if want := "start:a1 start:b1"; trace.got() != want {
		t.Fatalf("after first Apply, trace = %q, want %q", trace.got(), want)
	}

	if err := m.Apply(mgrChain(mgrLeaf2ID, mgrTop2ID, string(mod2))); err != nil {
		t.Fatalf("second Apply: %v", err)
	}
	// New tree starts first, then the old tree stops in reverse order.
	if want := "start:a1 start:b1 start:c1 start:d1 stop:b1 stop:a1"; trace.got() != want {
		t.Fatalf("after second Apply, trace = %q, want %q", trace.got(), want)
	}
}

// TestManagerApplyStartFailureKeepsOld verifies an Apply whose new Runtime
// fails to start leaves the old Runtime running and untouched.
func TestManagerApplyStartFailureKeepsOld(t *testing.T) {
	const mod1 = ModuleID("fe.test.manager.startfail.t1")
	const modFail = ModuleID("fe.test.manager.startfail.t2")
	boom := errors.New("start boom")
	trace := new(lifecycleTrace)
	registerMgrMod(t, mod1, map[string]string{mgrLeaf1ID: "a1", mgrTop1ID: "b1"}, trace, nil, nil)
	// Only the chain top (d1) fails to start, so the leaf (c1) starts first
	// and must be rolled back.
	RegisterModule(provMod(modFail, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		inst := &mgrInstance{name: map[string]string{mgrLeaf2ID: "c1", mgrTop2ID: "d1"}[spec.InstanceID], trace: trace}
		if spec.InstanceID == mgrTop2ID {
			inst.failStart = boom
		}
		return inst, nil
	}))

	m := new(Manager)
	if err := m.Apply(mgrChain(mgrLeaf1ID, mgrTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	old := m.current

	err := m.Apply(mgrChain(mgrLeaf2ID, mgrTop2ID, string(modFail)))
	if err == nil {
		t.Fatal("Apply: expected error from failing new tree")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Apply error = %v, want errors.Is(err, boom)", err)
	}
	if m.current != old {
		t.Fatal("Apply failure must not swap Manager.current")
	}
	// New tree's leaf started then rolled back; the old tree was never stopped.
	if want := "start:a1 start:b1 start:c1 stop:c1"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
}

// TestManagerApplyBuildFailureKeepsOld verifies an Apply whose new Runtime
// fails during construction (Provision) leaves the old one untouched.
func TestManagerApplyBuildFailureKeepsOld(t *testing.T) {
	const mod1 = ModuleID("fe.test.manager.buildfail.t1")
	const modBad = ModuleID("fe.test.manager.buildfail.t2")
	boom := errors.New("provision boom")
	trace := new(lifecycleTrace)
	registerMgrMod(t, mod1, map[string]string{mgrLeaf1ID: "a1", mgrTop1ID: "b1"}, trace, nil, nil)
	RegisterModule(provMod(modBad, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return nil, boom
	}))

	m := new(Manager)
	if err := m.Apply(mgrChain(mgrLeaf1ID, mgrTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	old := m.current

	err := m.Apply(mgrChain(mgrLeaf2ID, mgrTop2ID, string(modBad)))
	if err == nil {
		t.Fatal("Apply: expected error from failing provision")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("Apply error = %v, want errors.Is(err, boom)", err)
	}
	if m.current != old {
		t.Fatal("Apply failure must not swap Manager.current")
	}
	if want := "start:a1 start:b1"; trace.got() != want {
		t.Fatalf("old tree was disturbed: trace = %q, want %q", trace.got(), want)
	}
}

// TestManagerApplyOldStopErrorIgnored verifies a Stop error from the old tree
// during swap is logged/best-effort and does not fail Apply.
func TestManagerApplyOldStopErrorIgnored(t *testing.T) {
	const mod1 = ModuleID("fe.test.manager.stoperr.t1")
	const mod2 = ModuleID("fe.test.manager.stoperr.t2")
	stopBoom := errors.New("old stop boom")
	trace := new(lifecycleTrace)
	registerMgrMod(t, mod1, map[string]string{mgrLeaf1ID: "a1", mgrTop1ID: "b1"}, trace, nil, stopBoom)
	registerMgrMod(t, mod2, map[string]string{mgrLeaf2ID: "c1", mgrTop2ID: "d1"}, trace, nil, nil)

	m := new(Manager)
	if err := m.Apply(mgrChain(mgrLeaf1ID, mgrTop1ID, string(mod1))); err != nil {
		t.Fatalf("first Apply: %v", err)
	}

	// Old tree's Stop fails, but Apply must still succeed (swap completed).
	if err := m.Apply(mgrChain(mgrLeaf2ID, mgrTop2ID, string(mod2))); err != nil {
		t.Fatalf("second Apply: expected nil despite old tree Stop failure, got %v", err)
	}
	if m.current == nil {
		t.Fatal("Manager.current must hold the new Runtime")
	}
	// Old tree's Stop failed before recording (failStop short-circuits), so
	// only the starts appear.
	if want := "start:a1 start:b1 start:c1 start:d1"; trace.got() != want {
		t.Fatalf("trace = %q, want %q", trace.got(), want)
	}
}

// TestManagerStop verifies Stop shuts the active Runtime down in reverse
// order, clears it, and is a no-op once nothing is active.
func TestManagerStop(t *testing.T) {
	const modID = ModuleID("fe.test.manager.stop")
	trace := new(lifecycleTrace)
	registerMgrMod(t, modID, map[string]string{mgrLeaf1ID: "a1", mgrTop1ID: "b1"}, trace, nil, nil)

	m := new(Manager)
	if err := m.Apply(mgrChain(mgrLeaf1ID, mgrTop1ID, string(modID))); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if want := "start:a1 start:b1"; trace.got() != want {
		t.Fatalf("after Apply, trace = %q, want %q", trace.got(), want)
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if want := "start:a1 start:b1 stop:b1 stop:a1"; trace.got() != want {
		t.Fatalf("after Stop, trace = %q, want %q", trace.got(), want)
	}
	if m.current != nil {
		t.Fatal("after Stop, Manager.current must be cleared")
	}

	// No active Runtime → second Stop is a no-op, not an error.
	if err := m.Stop(); err != nil {
		t.Fatalf("second Stop: expected nil (no active Runtime), got %v", err)
	}
	if want := "start:a1 start:b1 stop:b1 stop:a1"; trace.got() != want {
		t.Fatalf("second Stop changed trace: %q, want %q", trace.got(), want)
	}
}
