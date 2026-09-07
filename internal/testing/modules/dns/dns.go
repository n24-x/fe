// Package dns is a test fe module (module ID "dns") used to drive the
// framework design. It is a leaf: it depends on nothing, and dns.forwarder
// depends on it. It demonstrates an instance-producing module with config.
package dns

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

// Module is the dns module type. It is stateless and shared.
type Module struct{}

// FeModule implements fe.Module. No side-effects.
func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "dns"}
}

// config is the dns instance's config (a private DTO).
type config struct {
	EnableCache bool `json:"enable_cache"`
}

// Provision implements fe.Provisioner.
func (Module) Provision(spec feconfig.InstanceSpec, rt *fe.Runtime) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("dns: decoding config: %w", err)
		}
	}
	return &Instance{enableCache: cfg.EnableCache}, nil
}

// Instance is a dns module instance.
type Instance struct {
	enableCache bool
}

// Start implements fe.Instance.
func (i *Instance) Start() error {
	log.Printf("[dns] started (enable_cache=%v)", i.enableCache)
	return nil
}

// Stop implements fe.Instance.
func (i *Instance) Stop() error {
	log.Printf("[dns] stopped")
	return nil
}

// EnableCache reports whether the cache is enabled.
func (i *Instance) EnableCache() bool { return i.enableCache }
