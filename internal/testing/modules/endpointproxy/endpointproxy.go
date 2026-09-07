// Package endpointproxy is a test fe module (module ID
// "endpoint.proxy.server") used to drive the framework design. It depends on
// a dns.forwarder instance (its config carries that instance's id).
package endpointproxy

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

// Module is the endpoint.proxy.server module type. It is stateless and shared.
type Module struct{}

// FeModule implements fe.Module. No side-effects.
func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "endpoint.proxy.server"}
}

// config is the endpoint.proxy.server instance's config (a private DTO).
type config struct {
	// DomainResolver is the id of the dns.forwarder instance used to resolve
	// domain names.
	DomainResolver string `json:"domain_resolver"`
}

// Provision implements fe.Provisioner.
func (Module) Provision(spec feconfig.InstanceSpec, rt *fe.Runtime) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("endpoint.proxy.server: decoding config: %w", err)
		}
	}
	return &Instance{resolverID: cfg.DomainResolver}, nil
}

// Instance is an endpoint.proxy.server module instance.
type Instance struct {
	resolverID string // id of the dns.forwarder instance this depends on
}

// Start implements fe.Instance.
func (i *Instance) Start() error {
	log.Printf("[endpoint.proxy.server] started (resolver=%s)", i.resolverID)
	return nil
}

// Stop implements fe.Instance.
func (i *Instance) Stop() error {
	log.Printf("[endpoint.proxy.server] stopped")
	return nil
}

// ResolverID returns the id of the dns.forwarder instance this server uses.
func (i *Instance) ResolverID() string { return i.resolverID }
