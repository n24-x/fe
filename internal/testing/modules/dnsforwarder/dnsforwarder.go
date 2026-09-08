// Package dnsforwarder is a test fe module (module ID "dns.forwarder") used
// to drive the framework design. It demonstrates the dependency-resolution
// pattern: its config names a dns instance by id, and Provision resolves it
// to a live *dns.Instance reference via rt.Instance (then type-asserts),
// which Start then uses.
package dnsforwarder

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/testing/modules/dns"
)

func init() {
	fe.RegisterModule(Module{})
}

// Module is the dns.forwarder module type. It is stateless and shared.
type Module struct{}

// FeModule implements fe.Module. No side-effects.
func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "dns.forwarder"}
}

// config is the dns.forwarder instance's config (a private DTO).
type config struct {
	// DNSInst is the id of the dns instance this forwarder depends on.
	DNSInst string `json:"dns_inst"`
	// Upstream is the upstream DNS server to forward to.
	Upstream string `json:"upstream"`
}

// Provision implements fe.Provisioner: it parses the config, resolves the
// dependency (rt.Instance) and type-asserts it to the concrete *dns.Instance
// it needs. dns_inst is required — a forwarder without a dns instance to
// forward to is a configuration error.
func (Module) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("dns.forwarder: decoding config: %w", err)
		}
	}
	if cfg.DNSInst == "" {
		return nil, fmt.Errorf("dns.forwarder: config dns_inst is required")
	}
	if cfg.Upstream == "" {
		cfg.Upstream = "1.1.1.1" // default
	}

	dep, err := rt.Instance(cfg.DNSInst)
	if err != nil {
		return nil, fmt.Errorf("dns.forwarder: resolving dns instance %q: %w", cfg.DNSInst, err)
	}
	dnsInst, ok := dep.(*dns.Instance)
	if !ok {
		return nil, fmt.Errorf("dns.forwarder: dep %q is a %T, want *dns.Instance", cfg.DNSInst, dep)
	}

	return &Instance{dns: dnsInst, upstream: cfg.Upstream}, nil
}

// Instance is a dns.forwarder module instance.
type Instance struct {
	dns      *dns.Instance // resolved dependency: the dns instance to forward to
	upstream string
}

// Start implements fe.Instance.
func (i *Instance) Start() error {
	log.Printf("[dns.forwarder] started (dns_cache=%v upstream=%s)", i.dns.EnableCache(), i.upstream)
	return nil
}

// Stop implements fe.Instance.
func (i *Instance) Stop() error {
	log.Printf("[dns.forwarder] stopped")
	return nil
}

// DNS returns the resolved dns instance this forwarder forwards to.
func (i *Instance) DNS() *dns.Instance { return i.dns }

// Upstream returns the configured upstream DNS server.
func (i *Instance) Upstream() string { return i.upstream }
