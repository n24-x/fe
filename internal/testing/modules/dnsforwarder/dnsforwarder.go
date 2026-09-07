// Package dnsforwarder is a test fe module (module ID "dns.forwarder") used
// to drive the framework design. It depends on a dns instance: its config
// carries the dns instance's id, which the runtime resolves into a reference.
// The id is stored as-is for now; resolving it to a live instance via
// Runtime.Instance during Provision is a later step.
package dnsforwarder

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
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

// Provision implements fe.Provisioner.
func (Module) Provision(spec feconfig.InstanceSpec, rt *fe.Runtime) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("dns.forwarder: decoding config: %w", err)
		}
	}
	if cfg.Upstream == "" {
		cfg.Upstream = "1.1.1.1" // default
	}
	return &Instance{dnsID: cfg.DNSInst, upstream: cfg.Upstream}, nil
}

// Instance is a dns.forwarder module instance.
type Instance struct {
	dnsID    string // id of the dns instance this depends on (resolution not wired yet)
	upstream string
}

// Start implements fe.Instance.
func (i *Instance) Start() error {
	log.Printf("[dns.forwarder] started (dns=%s upstream=%s)", i.dnsID, i.upstream)
	return nil
}

// Stop implements fe.Instance.
func (i *Instance) Stop() error {
	log.Printf("[dns.forwarder] stopped")
	return nil
}

// DNSID returns the id of the dns instance this forwarder depends on.
func (i *Instance) DNSID() string { return i.dnsID }

// Upstream returns the configured upstream DNS server.
func (i *Instance) Upstream() string { return i.upstream }
