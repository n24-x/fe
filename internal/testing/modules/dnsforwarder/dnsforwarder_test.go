package dnsforwarder

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/testing/modules/dns"
)

// fakeRT is a minimal fe.RuntimeAccess fake for module tests: Instance
// returns a canned dependency (or error), Context a background context.
type fakeRT struct {
	inst fe.Instance
	err  error
}

func (f fakeRT) Instance(id string) (fe.Instance, error) { return f.inst, f.err }
func (f fakeRT) Context() context.Context                { return context.Background() }

// stubDep is an Instance that is NOT *dns.Instance, for the wrong-type case.
type stubDep struct{}

func (stubDep) Start() error { return nil }
func (stubDep) Stop() error  { return nil }

func TestProvision(t *testing.T) {
	const dnsInstID = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"

	dnsInst := new(dns.Instance) // EnableCache() defaults to false
	wantErr := errors.New("dep not found")
	lookupErr := errors.New("dep lookup failed")

	full := func(upstream string) string {
		return `{"dns_inst": "` + dnsInstID + `", "upstream": "` + upstream + `"}`
	}

	tests := []struct {
		name         string
		config       string // "" = no config section
		rt           fe.RuntimeAccess
		wantUpstream string
		wantDNS      *dns.Instance
		wantErrSub   string // substring expected in the error; "" = must succeed
	}{
		{
			name:         "resolves dep and parses config",
			config:       full("8.8.8.8"),
			rt:           fakeRT{inst: dnsInst},
			wantUpstream: "8.8.8.8",
			wantDNS:      dnsInst,
		},
		{
			name:         "upstream omitted defaults",
			config:       `{"dns_inst": "` + dnsInstID + `"}`,
			rt:           fakeRT{inst: dnsInst},
			wantUpstream: "1.1.1.1",
			wantDNS:      dnsInst,
		},
		{
			name:       "dns_inst missing is rejected",
			config:     `{"upstream": "8.8.8.8"}`,
			rt:         fakeRT{},
			wantErrSub: "dns_inst is required",
		},
		{
			name:       "dep lookup error is propagated",
			config:     full("8.8.8.8"),
			rt:         fakeRT{err: lookupErr},
			wantErrSub: lookupErr.Error(),
		},
		{
			name:       "dep of wrong type is rejected",
			config:     full("8.8.8.8"),
			rt:         fakeRT{inst: stubDep{}},
			wantErrSub: "want *dns.Instance",
		},
		{
			name:       "malformed config is rejected",
			config:     `{"dns_inst": `,
			rt:         fakeRT{inst: dnsInst},
			wantErrSub: "decoding config",
		},
		{
			name:       "lookup error wins over returned inst",
			config:     full("8.8.8.8"),
			rt:         fakeRT{inst: dnsInst, err: wantErr},
			wantErrSub: wantErr.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := feconfig.InstanceSpec{ModuleID: "dns.forwarder"}
			if tt.config != "" {
				spec.Config = json.RawMessage(tt.config)
			}

			inst, err := (Module{}).Provision(spec, tt.rt)
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

			fwd, ok := inst.(*Instance)
			if !ok {
				t.Fatalf("Provision returned %T, want *Instance", inst)
			}
			if got := fwd.Upstream(); got != tt.wantUpstream {
				t.Errorf("Upstream() = %q, want %q", got, tt.wantUpstream)
			}
			if tt.wantDNS != nil && fwd.DNS() != tt.wantDNS {
				t.Errorf("DNS() = %p, want %p (the resolved dns instance)", fwd.DNS(), tt.wantDNS)
			}
		})
	}
}
