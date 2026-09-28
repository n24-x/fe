package dns

import (
	"encoding/json"
	"fmt"

	"github.com/n24-x/fe"
	"github.com/n24-x/fe/feconfig"
	"github.com/n24-x/fe/internal/demo/modules/logger"
)

func init() {
	fe.RegisterModule(Module{})
}

type Module struct{}

var (
	_ fe.Module      = Module{}
	_ fe.Provisioner = Module{}
)

func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "dns"}
}

type config struct {
	LoggerInst  string `json:"logger_inst"`
	EnableCache bool   `json:"enable_cache"`
}

func (Module) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("dns: decoding config: %w", err)
		}
	}
	if cfg.LoggerInst == "" {
		return nil, fmt.Errorf("dns: config logger_inst is required")
	}

	dep, err := rt.Instance(cfg.LoggerInst)
	if err != nil {
		return nil, fmt.Errorf("dns: resolving logger instance %q: %w", cfg.LoggerInst, err)
	}
	logs, ok := dep.(logger.LogProvider)
	if !ok {
		return nil, fmt.Errorf("dns: logger_inst %q is a %T, want a logger.LogProvider", cfg.LoggerInst, dep)
	}

	// The dependency is stored, not its logger: the logger only exists after
	// that instance has started, and the framework starts dependencies first.
	return &Instance{enableCache: cfg.EnableCache, logs: logs}, nil
}

type Instance struct {
	enableCache bool
	logs        logger.LogProvider
}

var _ fe.Instance = (*Instance)(nil)

func (i *Instance) Start() error {
	i.logs.Named("dns").Info("[dns] started")
	return nil
}

func (i *Instance) Stop() error {
	i.logs.Named("dns").Info("[dns] stopped")
	return nil
}

func (i *Instance) EnableCache() bool { return i.enableCache }
