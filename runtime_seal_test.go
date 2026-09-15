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
// the three read-only methods. It is the compile-time half of the seal: adding
// anything that drives the lifecycle (Start, Stop, Reload, …) to
// RuntimeAccess would put it in every module's hands, and this test fails
// first.
func TestRuntimeAccessSurface(t *testing.T) {
	it := reflect.TypeOf((*fe.RuntimeAccess)(nil)).Elem()

	var got []string
	for i := range it.NumMethod() {
		got = append(got, it.Method(i).Name)
	}
	sort.Strings(got)

	want := []string{"BusClient", "Context", "Instance"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RuntimeAccess methods = %v, want %v", got, want)
	}
}
