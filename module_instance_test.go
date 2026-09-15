package fe

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"uuid"

	"github.com/n24-x/fe/feconfig"
)

const testIDStringRaw = "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"

// TestInstanceIDString pins the canonical rendering. InstanceID is a defined
// type, so it does not inherit uuid.UUID's String method: without one,
// formatting it yields raw bytes rather than the uuid.
func TestInstanceIDString(t *testing.T) {
	id := InstanceID(uuid.MustParse(testIDStringRaw))

	if got := id.String(); got != testIDStringRaw {
		t.Errorf("String() = %q, want %q", got, testIDStringRaw)
	}
	for _, verb := range []string{"%s", "%v"} {
		if got := fmt.Sprintf(verb, id); got != testIDStringRaw {
			t.Errorf("%s = %q, want %q", verb, got, testIDStringRaw)
		}
	}
}

// TestRuntimeErrorsNameTheInstance verifies the framework's own error messages
// carry the instance id in the form it was written in the config.
//
// The lifecycle paths interpolate an InstanceID with %s, which renders raw
// bytes unless InstanceID has a String method — an unreadable error on exactly
// the paths (failed Start, failed Stop) where a user most needs to know which
// instance misbehaved. The assertions are on the message text on purpose:
// errors.Is cannot see this.
func TestRuntimeErrorsNameTheInstance(t *testing.T) {
	const modID = ModuleID("fe.test.idfmt")
	const idFailStop = "7c1b4a6e-8f0d-4c9e-9a2b-1c3d4e5f6a7b"
	startBoom := errors.New("bind failed")
	stopBoom := errors.New("flush failed")

	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		inst := &traceInstance{name: "id", trace: new(lifecycleTrace)}
		switch spec.InstanceID {
		case testIDStringRaw:
			inst.startErr = startBoom
		case idFailStop:
			inst.stopErr = stopBoom
		}
		return inst, nil
	}))

	// one builds a Runtime over the given instances (deps first, as the
	// config lists them).
	one := func(t *testing.T, ids ...string) *Runtime {
		t.Helper()
		specs := make([]feconfig.InstanceSpec, 0, len(ids))
		for i, id := range ids {
			spec := feconfig.InstanceSpec{InstanceID: id, ModuleID: string(modID)}
			if i > 0 {
				spec.Deps = []string{ids[i-1]}
			}
			specs = append(specs, spec)
		}
		rt, err := NewRuntime(&feconfig.MachineConfig{Instances: specs})
		if err != nil {
			t.Fatalf("NewRuntime: %v", err)
		}
		return rt
	}

	assertNames := func(t *testing.T, what string, err error, ids ...string) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected an error", what)
		}
		for _, id := range ids {
			if !strings.Contains(err.Error(), id) {
				t.Errorf("%s error = %q, want it to contain the instance id %q", what, err, id)
			}
		}
	}

	// Failed Start names the failing instance.
	rt := one(t, testIDStringRaw)
	err := rt.Start()
	if !errors.Is(err, startBoom) {
		t.Fatalf("Start error = %v, want errors.Is(err, startBoom)", err)
	}
	assertNames(t, "Start", err, testIDStringRaw)
	rt.Stop()

	// Failed Stop names the instance on the teardown path.
	rt = one(t, idFailStop)
	if err := rt.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err = rt.Stop()
	if !errors.Is(err, stopBoom) {
		t.Fatalf("Stop error = %v, want errors.Is(err, stopBoom)", err)
	}
	assertNames(t, "Stop", err, idFailStop)

	// A rolled-back Start names both: the instance that failed to start and
	// the already-started one whose rollback Stop then failed.
	rt = one(t, idFailStop, testIDStringRaw)
	err = rt.Start()
	if !errors.Is(err, startBoom) || !errors.Is(err, stopBoom) {
		t.Fatalf("rollback error = %v, want it to join both start and stop failures", err)
	}
	assertNames(t, "rollback", err, testIDStringRaw, idFailStop)
	rt.Stop()
}
