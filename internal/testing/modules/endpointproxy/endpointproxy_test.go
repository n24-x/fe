package endpointproxy

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/eventbus"
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

// Done completes the interface; a fake Runtime is never stopped, so the signal
// it hands out is never closed (and a nil channel never fires in a select).
func (f fakeRT) Done() <-chan struct{} { return nil }

// BusClient completes the interface; endpointproxy does not use the event bus,
// so a call would be a bug in the module under test.
func (f fakeRT) BusClient(name string) (*eventbus.Client, error) {
	return nil, errors.New("fakeRT: endpointproxy must not use the event bus")
}

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

// testFwdID is the dns.forwarder instance id the proxy configs below refer to
// through domain_resolver.
const testFwdID = "7b1d2a5e-9f4c-4a8b-8c3d-2e5f1a6b7c8d"

// provisionProxy builds a server the way NewRuntime would: through the
// module's Provision, with the dns.forwarder dependency resolved by fakeRT.
func provisionProxy(t *testing.T, host string, port int) *Instance {
	t.Helper()
	spec := feconfig.InstanceSpec{
		ModuleID: "endpoint.proxy.server",
		Config: json.RawMessage(`{"listen": "` + host + `", "port": ` + strconv.Itoa(port) +
			`, "domain_resolver": "` + testFwdID + `"}`),
	}
	inst, err := (Module{}).Provision(spec, fakeRT{inst: newFwd(t)})
	if err != nil {
		t.Fatalf("provisioning endpoint.proxy.server: %v", err)
	}
	return inst.(*Instance)
}

// freePort returns a TCP port that was free when it was probed: the probe
// listener is closed again before returning, so the instance can take it (the
// small race in between is one a test can live with).
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probing a free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// TestInstanceStartStop covers the listener lifecycle, the one part of this
// module that owns a real resource: Start binds the configured address and
// serves (echoing every line), Stop closes the listener and frees the port,
// and Stop is idempotent so the Runtime's teardown can call it unconditionally.
func TestInstanceStartStop(t *testing.T) {
	port := freePort(t)
	inst := provisionProxy(t, "127.0.0.1", port)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	if err := inst.Start(); err != nil {
		t.Fatalf("Start: unexpected error: %v", err)
	}

	// The accept loop must serve: every line written comes back.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dialing the started instance at %s: %v", addr, err)
	}
	if _, err := conn.Write([]byte("hello\n")); err != nil {
		t.Fatalf("writing to the instance: %v", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("reading the echo: %v", err)
	}
	if line != "hello\n" {
		t.Fatalf("echo = %q, want %q", line, "hello\n")
	}
	conn.Close()

	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}
	if err := inst.Stop(); err != nil {
		t.Fatalf("second Stop: unexpected error: %v (Stop must be idempotent)", err)
	}

	// The port is available again, i.e. the listener was really closed.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("re-binding %s after Stop: %v", addr, err)
	}
	ln.Close()
}

// TestInstanceStopBeforeStart verifies Stop is safe on an instance that never
// started — Stop before Start, or a rolled-back Start — where the listener is
// still nil.
func TestInstanceStopBeforeStart(t *testing.T) {
	inst := provisionProxy(t, "127.0.0.1", freePort(t))

	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop before Start: unexpected error: %v", err)
	}
}

// TestInstanceStartBindFailure verifies Start surfaces a bind failure instead
// of leaving a half-started instance behind (the Runtime rolls the whole
// config back on it), and that the failed instance is still safe to Stop.
func TestInstanceStartBindFailure(t *testing.T) {
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("binding a blocker listener: %v", err)
	}
	defer blocker.Close()

	port := blocker.Addr().(*net.TCPAddr).Port
	inst := provisionProxy(t, "127.0.0.1", port)

	err = inst.Start()
	if err == nil {
		t.Fatal("Start: expected a bind error, got nil")
	}
	if !strings.Contains(err.Error(), "listen on") {
		t.Fatalf("Start error = %v, want it to mention the address it could not bind", err)
	}
	if err := inst.Stop(); err != nil {
		t.Fatalf("Stop after a failed Start: unexpected error: %v", err)
	}
}
