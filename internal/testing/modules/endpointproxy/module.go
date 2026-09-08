// Package endpointproxy is a test fe module (module ID
// "endpoint.proxy.server") used to drive the framework design. It depends on
// a dns.forwarder instance: its config names that instance by id, and
// Provision resolves it to a live *dnsforwarder.Instance via rt.Instance,
// which Start then uses. Same dependency-resolution pattern as dns.forwarder.
package endpointproxy

import (
	"encoding/json"
	"fmt"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/testing/modules/dnsforwarder"
)

func init() {
	fe.RegisterModule(Module{})
}

// Module is the endpoint.proxy.server module type. It is stateless and shared.
type Module struct{}

// FeModule implements fe.Module. No side-effects.
func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "endpoint.proxy.server"}
}

// config is the endpoint.proxy.server instance's config (a private DTO).
type config struct {
	// Listen is the address to bind the TCP proxy to.
	Listen string `json:"listen"`
	// Port is the TCP port to listen on.
	Port int `json:"port"`
	// DomainResolver is the id of the dns.forwarder instance used to resolve
	// domain names.
	DomainResolver string `json:"domain_resolver"`
}

// Provision implements fe.Provisioner: it parses the config, resolves the
// dependency (rt.Instance) and type-asserts it to *dnsforwarder.Instance.
// domain_resolver is required — a proxy server without a resolver to use is
// a configuration error.
func (Module) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("endpoint.proxy.server: decoding config: %w", err)
		}
	}
	if cfg.DomainResolver == "" {
		return nil, fmt.Errorf("endpoint.proxy.server: config domain_resolver is required")
	}

	dep, err := rt.Instance(cfg.DomainResolver)
	if err != nil {
		return nil, fmt.Errorf("endpoint.proxy.server: resolving dns.forwarder instance %q: %w", cfg.DomainResolver, err)
	}
	fwd, ok := dep.(*dnsforwarder.Instance)
	if !ok {
		return nil, fmt.Errorf("endpoint.proxy.server: dep %q is a %T, want *dnsforwarder.Instance", cfg.DomainResolver, dep)
	}

	return &Instance{
		listen: cfg.Listen,
		port:   cfg.Port,
		dns:    fwd,
	}, nil
}
