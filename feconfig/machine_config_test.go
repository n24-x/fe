package feconfig

import (
	"encoding/json"
	"errors"
	"testing"
)

// Instance ids used across the validation cases.
const (
	idA = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	idB = "7b1d2a5e-9f4c-4a8b-8c3d-2e5f1a6b7c8d"
	idC = "c0d1e2f3-4a5b-4c6d-9e0f-1a2b3c4d5e6f"
)

// spec builds an InstanceSpec; deps are optional.
func spec(id, mod string, deps ...string) InstanceSpec {
	return InstanceSpec{InstanceID: id, ModuleID: mod, Deps: deps}
}

// mc builds a MachineConfig from instance specs.
func mc(specs ...InstanceSpec) *MachineConfig {
	return &MachineConfig{Instances: specs}
}

// TestParseHelper covers the JSON -> value step, including strict decoding
// (unknown fields rejected) which is ParseHelper's responsibility alone.
func TestParseHelper(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr error // errors.Is target; nil means it must parse
	}{
		{
			name: "valid config",
			raw:  `{"options":{"bus":{"router_capacity":8}},"instances":[{"id":"` + idA + `","mod_id":"dns","config":{},"deps":[]}]}`,
		},
		{
			name: "options omitted",
			raw:  `{"instances":[]}`,
		},
		{
			name: "empty instances",
			raw:  `{}`,
		},
		{
			name:    "malformed json",
			raw:     `{"instances":`,
			wantErr: ErrMalformedConfig,
		},
		{
			name:    "unknown top-level field",
			raw:     `{"extra":1,"instances":[]}`,
			wantErr: ErrMalformedConfig,
		},
		{
			name:    "unknown instance field",
			raw:     `{"instances":[{"id":"` + idA + `","mod_id":"dns","config":{},"deps":[],"unexpected":true}]}`,
			wantErr: ErrMalformedConfig,
		},
		{
			name:    "unknown options field",
			raw:     `{"options":{"bogus":1},"instances":[]}`,
			wantErr: ErrMalformedConfig,
		},
		{
			name:    "unknown bus option field",
			raw:     `{"options":{"bus":{"bogus":1}},"instances":[]}`,
			wantErr: ErrMalformedConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHelper(json.RawMessage(tt.raw))
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want errors.Is(err, %v), got err = %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("ParseHelper returned nil config without error")
			}
		})
	}
}

// TestParseHelperOptions verifies the framework facility options survive the
// JSON round trip.
func TestParseHelperOptions(t *testing.T) {
	mc, err := ParseHelper(json.RawMessage(`{"options":{"bus":{"router_capacity":8}},"instances":[]}`))
	if err != nil {
		t.Fatalf("ParseHelper: %v", err)
	}
	if got := mc.Options.Bus.RouterCapacity; got != 8 {
		t.Fatalf("Options.Bus.RouterCapacity = %d, want 8", got)
	}
}

// TestMachineConfigValidate covers the semantic checks on an in-process value.
// Cases are built directly in Go — no JSON is involved.
func TestMachineConfigValidate(t *testing.T) {
	const validID = "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e" // valid v4
	const nonV4ID = "3f2504e0-4f89-11d3-9a0c-0305e82c3301" // version bits = 1

	tests := []struct {
		name string
		mc   *MachineConfig
		want error // errors.Is target; nil means it must pass
	}{
		{
			name: "nil config",
			mc:   nil,
			want: ErrMalformedConfig,
		},
		{
			name: "empty instances",
			mc:   mc(),
		},
		{
			name: "valid dep chain",
			mc:   mc(spec(idA, "dns.resolver"), spec(idB, "dns.forwarder", idA), spec(idC, "http.static", idB)),
		},
		{
			name: "empty mod_id",
			mc:   mc(InstanceSpec{InstanceID: validID}),
			want: ErrMissingModuleID,
		},
		{
			name: "invalid uuid id",
			mc:   mc(spec("not-a-uuid", "dns.resolver")),
			want: ErrInvalidID,
		},
		{
			name: "id non-v4",
			mc:   mc(spec(nonV4ID, "dns.resolver")),
			want: ErrInvalidID,
		},
		{
			name: "duplicate id",
			mc:   mc(spec(idA, "dns.resolver"), spec(idA, "dns.forwarder")),
			want: ErrDuplicateID,
		},
		{
			name: "dep references undefined instance",
			mc:   mc(spec(idA, "dns.resolver", idB)),
			want: ErrUndefinedDep,
		},
		{
			name: "cyclic deps",
			mc:   mc(spec(idA, "dns.resolver", idB), spec(idB, "dns.forwarder", idA)),
			want: ErrCyclicDeps,
		},
		{
			name: "self dep cycle",
			mc:   mc(spec(idA, "dns.resolver", idA)),
			want: ErrCyclicDeps,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MachineConfigValidate(tt.mc)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("expected pass, got error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("want errors.Is(err, %v), got err = %v", tt.want, err)
			}
		})
	}
}
