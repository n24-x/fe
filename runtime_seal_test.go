package fe_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
)

// This file is the *external* view of the framework (package fe_test sits
// outside package fe, so it can name only exported identifiers — exactly the
// position a third-party module author is in). It pins the seal on
// RuntimeAccess: what Provision receives must not let the module drive the
// Runtime's lifecycle.

// sealMod is a module whose only job is to hand the RuntimeAccess it receives
// back to the test.
type sealMod struct{}

func (sealMod) FeModule() fe.ModuleInfo { return fe.ModuleInfo{ID: "fe.test.seal"} }

func (sealMod) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	sealCapture = rt
	return sealInst{}, nil
}

// sealCapture holds the RuntimeAccess the last Provision call received.
var sealCapture fe.RuntimeAccess

type sealInst struct{}

func (sealInst) Start() error { return nil }
func (sealInst) Stop() error  { return nil }

// TestRuntimeAccessSealed verifies a module cannot recover the concrete
// Runtime from the RuntimeAccess it is given. Handing over *Runtime would
// satisfy the interface but leak Start/Stop through a one-line type assertion,
// letting a module end the lifecycle the framework owns (issue.md D30).
func TestRuntimeAccessSealed(t *testing.T) {
	fe.RegisterModule(sealMod{})

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e", ModuleID: "fe.test.seal"},
		},
	}
	rt, err := fe.NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: unexpected error: %v", err)
	}
	defer rt.Stop()

	if sealCapture == nil {
		t.Fatal("Provision never ran: nothing to inspect")
	}
	if concrete, ok := sealCapture.(*fe.Runtime); ok {
		t.Fatalf("Provision received %T: a module can call Start/Stop on it and drive the lifecycle", concrete)
	}
}

// TestRuntimeAccessSurface verifies the module-visible surface stays exactly
// the three read-only methods, and that the channel it hands out stays
// receive-only.
//
// It is the compile-time half of the seal, guarding the two ways the surface
// could stop being read-only: adding a method that drives the lifecycle
// (Start, Stop, Reload, …) would put it in every module's hands, and widening
// Done to a bidirectional channel would let a module close the Runtime's
// lifecycle signal. Both fail here first.
func TestRuntimeAccessSurface(t *testing.T) {
	it := reflect.TypeOf((*fe.RuntimeAccess)(nil)).Elem()

	var got []string
	for i := range it.NumMethod() {
		got = append(got, it.Method(i).Name)
	}
	sort.Strings(got)

	want := []string{"BusClient", "Done", "Instance"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RuntimeAccess methods = %v, want %v", got, want)
	}

	// Direction is what makes the signal safe to hand out: only the sender of
	// a channel may close it, so a module holding <-chan struct{} cannot take
	// the Runtime down — the guarantee the unexported cancel func used to give.
	done, ok := it.MethodByName("Done")
	if !ok {
		t.Fatal("RuntimeAccess has no Done method")
	}
	if n := done.Type.NumOut(); n != 1 {
		t.Fatalf("Done returns %d values, want 1", n)
	}
	ch := done.Type.Out(0)
	if ch.Kind() != reflect.Chan {
		t.Fatalf("Done returns %s, want a channel", ch)
	}
	if ch.ChanDir() != reflect.RecvDir {
		t.Fatalf("Done returns %s, want receive-only (<-chan struct{}): a bidirectional channel lets a module close the Runtime's lifecycle signal", ch)
	}
	if elem := ch.Elem(); elem.Kind() != reflect.Struct || elem.NumField() != 0 {
		t.Fatalf("Done returns %s, want <-chan struct{}", ch)
	}
}
