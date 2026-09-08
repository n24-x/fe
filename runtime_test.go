package fe

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"uuid"

	"github.com/n24-x/fe/feconfig"
)

// TestValidateRuntimeConfigRegistered verifies the validation passes when all
// mod_ids in the config are registered AND produce instances (Provisioner).
func TestValidateRuntimeConfigRegistered(t *testing.T) {
	const modID = ModuleID("fe.test.validate.ok")
	RegisterModule(provMod(modID, nil)) // registered via the fake from module_test.go helpers

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: "1b4e28ba-2fa1-11d2-883f-0016d3cca427", ModuleID: string(modID)},
			{InstanceID: "7b1d2a5e-9f4c-4a8b-8c3d-2e5f1a6b7c8d", ModuleID: string(modID)},
		},
	}
	if err := ValidateRuntimeConfig(mc); err != nil {
		t.Fatalf("ValidateRuntimeConfig: unexpected error: %v", err)
	}
}

// TestValidateRuntimeConfigUnregistered verifies the validation fails when a
// mod_id is not registered, and that the error pinpoints the offending
// instance.
func TestValidateRuntimeConfigUnregistered(t *testing.T) {
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{
				InstanceID: "1b4e28ba-2fa1-11d2-883f-0016d3cca427",
				ModuleID:   "fe.test.does.not.exist",
			},
		},
	}
	err := ValidateRuntimeConfig(mc)
	if err == nil {
		t.Fatal("ValidateRuntimeConfig: expected error for unregistered mod_id")
	}
	if !strings.Contains(err.Error(), "fe.test.does.not.exist") {
		t.Fatalf("ValidateRuntimeConfig error = %v, want it to mention the unregistered mod_id", err)
	}
	if !strings.Contains(err.Error(), "instances[0]") {
		t.Fatalf("ValidateRuntimeConfig error = %v, want it to mention instances[0]", err)
	}
}

// TestValidateRuntimeConfigNotProvisioner verifies the validation fails when a
// registered module does not implement Provisioner (instance-less module).
func TestValidateRuntimeConfigNotProvisioner(t *testing.T) {
	const modID = ModuleID("fe.test.validate.noprov")
	RegisterModule(mod(modID)) // fakeMod: registered but no Provisioner

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: "1b4e28ba-2fa1-11d2-883f-0016d3cca427", ModuleID: string(modID)},
		},
	}
	err := ValidateRuntimeConfig(mc)
	if err == nil {
		t.Fatal("ValidateRuntimeConfig: expected error for module without Provisioner")
	}
	if !errors.Is(err, ErrModuleNotProvisioner) {
		t.Fatalf("ValidateRuntimeConfig error = %v, want errors.Is(err, ErrModuleNotProvisioner)", err)
	}
}

// fakeConfigInstance is an Instance produced by the test Provisioner below.
type fakeConfigInstance struct {
	Level string
}

func (*fakeConfigInstance) Start() error { return nil }
func (*fakeConfigInstance) Stop() error  { return nil }

// TestNewRuntimeInstantiates verifies NewRuntime validates, then provisions
// every instance in order via the module's Provisioner (config parsing is the
// module's job), and fills instances/mods.
func TestNewRuntimeInstantiates(t *testing.T) {
	const modID = ModuleID("fe.test.newruntime.inst")
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		var cfg struct {
			Level string `json:"level"`
		}
		if len(spec.Config) > 0 {
			if err := json.Unmarshal(spec.Config, &cfg); err != nil {
				return nil, err
			}
		}
		return &fakeConfigInstance{Level: cfg.Level}, nil
	}))

	ids := []string{
		"9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e",
		"7c1b4a6e-8f0d-4c9e-9a2b-1c3d4e5f6a7b",
	}
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: ids[0], ModuleID: string(modID), Config: json.RawMessage(`{"level":"debug"}`)},
			{InstanceID: ids[1], ModuleID: string(modID), Config: json.RawMessage(`{}`)},
		},
	}

	r, err := NewRuntime(mc)
	if err != nil {
		t.Fatalf("NewRuntime: unexpected error: %v", err)
	}
	if len(r.instances) != 2 {
		t.Fatalf("NewRuntime: got %d instances, want 2", len(r.instances))
	}
	if !r.mods[modID] {
		t.Fatalf("NewRuntime: mods missing %q", modID)
	}

	inst, ok := r.instances[InstanceID(uuid.MustParse(ids[0]))]
	if !ok {
		t.Fatalf("NewRuntime: instance %q not in map", ids[0])
	}
	cfgInst, ok := inst.(*fakeConfigInstance)
	if !ok {
		t.Fatalf("NewRuntime: instance has type %T, want *fakeConfigInstance", inst)
	}
	if cfgInst.Level != "debug" {
		t.Fatalf("NewRuntime: config not parsed by Provision, Level = %q, want %q", cfgInst.Level, "debug")
	}
}

// TestNewRuntimeProvisionError verifies an error returned by a module's
// Provision aborts NewRuntime.
func TestNewRuntimeProvisionError(t *testing.T) {
	const modID = ModuleID("fe.test.newruntime.proverr")
	wantErr := errors.New("boom")
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return nil, wantErr
	}))

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{
				InstanceID: "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e",
				ModuleID:   string(modID),
				Config:     json.RawMessage(`{"bogus":1}`),
			},
		},
	}
	_, err := NewRuntime(mc)
	if err == nil {
		t.Fatal("NewRuntime: expected error from Provision")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("NewRuntime error = %v, want errors.Is(err, wantErr)", err)
	}
}

// TestNewRuntimeUnregisteredModule verifies NewRuntime rejects a config whose
// mod_id is not registered (semantic validation as its first stage).
func TestNewRuntimeUnregisteredModule(t *testing.T) {
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{
				InstanceID: "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e",
				ModuleID:   "fe.test.newruntime.missing",
			},
		},
	}
	_, err := NewRuntime(mc)
	if err == nil {
		t.Fatal("NewRuntime: expected error for unregistered module")
	}
	if !errors.Is(err, ErrModuleNotRegistered) {
		t.Fatalf("NewRuntime error = %v, want errors.Is(err, ErrModuleNotRegistered)", err)
	}
}

// TestInstanceOrder verifies the creation order derived from the dependency
// graph: every instance comes after all of its deps, regardless of the order
// the instances are listed in the config; and every instance is covered.
func TestInstanceOrder(t *testing.T) {
	const (
		a = "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e" // leaf
		b = "7c1b4a6e-8f0d-4c9e-9a2b-1c3d4e5f6a7b" // depends on a
		c = "5a6b7c8d-9e0f-4a1b-8c2d-3e4f5a6b7c8d" // depends on b
		d = "1b4e28ba-2fa1-4d2f-883f-0016d3cca427" // isolated
	)
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: c, ModuleID: "fe.test.order", Deps: []string{b}},
			{InstanceID: d, ModuleID: "fe.test.order"},
			{InstanceID: b, ModuleID: "fe.test.order", Deps: []string{a}},
			{InstanceID: a, ModuleID: "fe.test.order"},
		},
	}

	order, err := instanceOrder(mc)
	if err != nil {
		t.Fatalf("instanceOrder: unexpected error: %v", err)
	}
	if len(order) != 4 {
		t.Fatalf("instanceOrder: got %d specs, want 4", len(order))
	}

	pos := make(map[string]int, len(order))
	for i, spec := range order {
		pos[spec.InstanceID] = i
	}
	// dep must come strictly before its dependent
	if !(pos[a] < pos[b] && pos[b] < pos[c]) {
		t.Fatalf("instanceOrder: chain order wrong, pos = %v", pos)
	}
	// every spec present
	for _, id := range []string{a, b, c, d} {
		if _, ok := pos[id]; !ok {
			t.Fatalf("instanceOrder: spec %q missing from order", id)
		}
	}
}

// TestNewRuntimeNilInstance verifies that a Provisioner returning a nil
// Instance with no error aborts NewRuntime (guards the instances map against
// nil values).
func TestNewRuntimeNilInstance(t *testing.T) {
	const modID = ModuleID("fe.test.newruntime.nilinst")
	RegisterModule(provMod(modID, func(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error) {
		return nil, nil // nil instance, nil error: a module bug
	}))

	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{
				InstanceID: "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e",
				ModuleID:   string(modID),
			},
		},
	}
	_, err := NewRuntime(mc)
	if err == nil {
		t.Fatal("NewRuntime: expected error for nil instance")
	}
	if !strings.Contains(err.Error(), "nil instance") {
		t.Fatalf("NewRuntime error = %v, want it to mention the nil instance", err)
	}
}

// TestNewRuntimeCyclicDeps verifies NewRuntime rejects a config whose deps
// form a cycle even when it bypasses feconfig.MachineConfigValidate (the
// runtime must not trust its input ordering).
func TestNewRuntimeCyclicDeps(t *testing.T) {
	const modID = ModuleID("fe.test.newruntime.cycle")
	RegisterModule(provMod(modID, nil))

	a := "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e"
	b := "7c1b4a6e-8f0d-4c9e-9a2b-1c3d4e5f6a7b"
	mc := &feconfig.MachineConfig{
		Instances: []feconfig.InstanceSpec{
			{InstanceID: a, ModuleID: string(modID), Deps: []string{b}},
			{InstanceID: b, ModuleID: string(modID), Deps: []string{a}},
		},
	}
	_, err := NewRuntime(mc)
	if err == nil {
		t.Fatal("NewRuntime: expected error for cyclic deps")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("NewRuntime error = %v, want it to mention the cycle", err)
	}
}
