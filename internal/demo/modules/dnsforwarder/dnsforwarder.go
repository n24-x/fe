package dnsforwarder

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/demo/modules/dns"
)

func init() {
	fe.RegisterModule(Module{})
}

type Module struct{}

var _ fe.Module = Module{}

func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "dns.forwarder"}
}

type config struct {
	DNSInst  string `json:"dns_inst"`
	Upstream string `json:"upstream"`
}

var _ fe.Provisioner = Module{}

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

type Instance struct {
	dns      *dns.Instance
	upstream string
}

var _ fe.Instance = (*Instance)(nil)

func (i *Instance) Start() error {
	log.Printf("[dns.forwarder] started (dns_cache=%v upstream=%s)", i.dns.EnableCache(), i.upstream)
	return nil
}

func (i *Instance) Stop() error {
	log.Printf("[dns.forwarder] stopped")
	return nil
}

func (i *Instance) DNS() *dns.Instance { return i.dns }
func (i *Instance) Upstream() string   { return i.upstream }
