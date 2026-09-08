package endpointproxy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/testing/modules/dns"
	"github.com/n24-x/fe/internal/testing/modules/dnsforwarder"
)

// fakeRT is a minimal fe.RuntimeAccess fake (same shape as the one in
// dnsforwarder's tests): Instance returns a canned dependency (or error).
type fakeRT struct {
	inst fe.Instance
	err  error
}

func (f fakeRT) Instance(id string) (fe.Instance, error) { return f.inst, f.err }
func (f fakeRT) Context() context.Context                { return context.Background() }

// newFwd returns a real *dnsforwarder.Instance to serve as the resolved
// dependency: it is produced via the module's own Provision with a fake rt
// that resolves dns_inst to a fresh dns.Instance.
func newFwd(t *testing.T) *dnsforwarder.Instance {
	t.Helper()
	const dnsInstID = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	dnsInst := new(dns.Instance)
	dnsRT := fakeRT{inst: dnsInst}

	inst, err := (dnsforwarder.Module{}).Provision(feconfig.InstanceSpec{
		Config: json.RawMessage(`{"dns_inst": "` + dnsInstID + `"}`),
	}, dnsRT)
	if err != nil {
		t.Fatalf("building dns.forwarder dep: %v", err)
	}
	return inst.(*dnsforwarder.Instance)
}

// stubDep is an Instance that is NOT *dnsforwarder.Instance, for the
// wrong-type case.
type stubDep struct{}

func (stubDep) Start() error { return nil }
func (stubDep) Stop() error  { return nil }

func TestProvision(t *testing.T) {
	const fwdID = "7b1d2a5e-9f4c-4a8b-8c3d-2e5f1a6b7c8d"

	fwd := newFwd(t)
	lookupErr := errors.New("dep lookup failed")

	full := func() string {
		return `{"listen": "127.0.0.1", "port": 18080, "domain_resolver": "` + fwdID + `"}`
	}

	tests := []struct {
		name       string
		config     string // "" = no config section
		rt         fe.RuntimeAccess
		wantListen string
		wantPort   int
		wantDNS    *dnsforwarder.Instance
		wantErrSub string // substring expected in the error; "" = must succeed
	}{
		{
			name:       "resolves dep and parses config",
			config:     full(),
			rt:         fakeRT{inst: fwd},
			wantListen: "127.0.0.1",
			wantPort:   18080,
			wantDNS:    fwd,
		},
		{
			name:       "domain_resolver missing is rejected",
			config:     `{"listen": "127.0.0.1", "port": 18080}`,
			rt:         fakeRT{},
			wantErrSub: "domain_resolver is required",
		},
		{
			name:       "dep lookup error is propagated",
			config:     full(),
			rt:         fakeRT{err: lookupErr},
			wantErrSub: lookupErr.Error(),
		},
		{
			name:       "dep of wrong type is rejected",
			config:     full(),
			rt:         fakeRT{inst: stubDep{}},
			wantErrSub: "want *dnsforwarder.Instance",
		},
		{
			name:       "malformed config is rejected",
			config:     `{"listen": `,
			rt:         fakeRT{inst: fwd},
			wantErrSub: "decoding config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := feconfig.InstanceSpec{ModuleID: "endpoint.proxy.server"}
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

			proxy, ok := inst.(*Instance)
			if !ok {
				t.Fatalf("Provision returned %T, want *Instance", inst)
			}
			if tt.wantDNS != nil && proxy.DNS() != tt.wantDNS {
				t.Errorf("DNS() = %p, want %p (the resolved dns.forwarder instance)", proxy.DNS(), tt.wantDNS)
			}
		})
	}
}
