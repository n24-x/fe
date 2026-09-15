package logger

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/n24-x/fe/feconfig"
)

// TestProvision covers the module's half of the contract: parsing spec.Config
// (the framework does not decode it) into a fresh Instance, defaulting the
// level when the config does not name one.
//
// logger has no dependencies, so Provision ignores the RuntimeAccess; the
// specs below pass nil to keep that explicit.
func TestProvision(t *testing.T) {
	tests := []struct {
		name       string
		config     string // "" = no config section
		wantLevel  string
		wantErrSub string // substring expected in the error; "" = must succeed
	}{
		{
			name:      "no config section defaults to info",
			wantLevel: "info",
		},
		{
			name:      "empty config object defaults to info",
			config:    `{}`,
			wantLevel: "info",
		},
		{
			name:      "explicit level is parsed",
			config:    `{"level": "debug"}`,
			wantLevel: "debug",
		},
		{
			name:       "malformed config is rejected",
			config:     `{"level": `,
			wantErrSub: "decoding config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := feconfig.InstanceSpec{ModuleID: "logger"}
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

			l, ok := inst.(*Instance)
			if !ok {
				t.Fatalf("Provision returned %T, want *Instance", inst)
			}
			if got := l.Level(); got != tt.wantLevel {
				t.Errorf("Level() = %q, want %q", got, tt.wantLevel)
			}
		})
	}
}

// TestInstanceStartStop verifies the lifecycle contract every Instance shares:
// Start and Stop report no error. logger owns no resources, so its Start is
// only a hook the Runtime can order.
func TestInstanceStartStop(t *testing.T) {
	inst := new(Instance)

	if err := inst.Start(); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}
	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
}
