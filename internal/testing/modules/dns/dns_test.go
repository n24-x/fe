package dns

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

// TestProvision covers the module's half of the contract: parsing spec.Config
// (the framework does not decode it) into a fresh Instance, with an absent
// config section falling back to defaults.
//
// dns is a leaf — it depends on nothing — so Provision ignores the
// RuntimeAccess; the specs below pass nil to keep that explicit.
func TestProvision(t *testing.T) {
	tests := []struct {
		name       string
		config     string // "" = no config section
		wantCache  bool
		wantErrSub string // substring expected in the error; "" = must succeed
	}{
		{
			name: "no config section defaults to cache disabled",
		},
		{
			name:   "empty config object defaults to cache disabled",
			config: `{}`,
		},
		{
			name:      "enable_cache true is parsed",
			config:    `{"enable_cache": true}`,
			wantCache: true,
		},
		{
			name:      "enable_cache false is parsed",
			config:    `{"enable_cache": false}`,
			wantCache: false,
		},
		{
			name:       "malformed config is rejected",
			config:     `{"enable_cache": `,
			wantErrSub: "decoding config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := feconfig.InstanceSpec{ModuleID: "dns"}
			if tt.config != "" {
				spec.Config = json.RawMessage(tt.config)
			}

			inst, err := (Module{}).Provision(spec, nil)
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatal("Provision: expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Fatalf("Provision error = %v, want it to contain %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("Provision: unexpected error: %v", err)
			}

			d, ok := inst.(*Instance)
			if !ok {
				t.Fatalf("Provision returned %T, want *Instance", inst)
			}
			if got := d.EnableCache(); got != tt.wantCache {
				t.Errorf("EnableCache() = %v, want %v", got, tt.wantCache)
			}
		})
	}
}

// TestInstanceStartStop verifies the lifecycle contract every Instance shares:
// Start and Stop report no error. dns owns no resources, so its Start is only
// a hook the Runtime can order.
func TestInstanceStartStop(t *testing.T) {
	inst := new(Instance)

	if err := inst.Start(); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}
