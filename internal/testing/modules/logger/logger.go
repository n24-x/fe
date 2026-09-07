// Package logger is an example fe module used to drive the framework design.
//
// It demonstrates the module shape: a stateless Module type (registered in
// init, providing its identity), which implements Provisioner to build a
// fresh Instance per config spec. Config parsing is the module's own job —
// the framework does not decode spec.Config.
package logger

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

// Module is the logger module type. It is stateless and shared.
type Module struct{}

// FeModule implements fe.Module. No side-effects.
func (Module) FeModule() fe.ModuleInfo {
	return fe.ModuleInfo{ID: "logger"}
}

// config is the logger instance's config (a private DTO).
type config struct {
	Level string `json:"level"`
}

// Provision implements fe.Provisioner: it parses the spec's config and
// returns a fresh, fully-formed Instance.
func (Module) Provision(spec feconfig.InstanceSpec, rt *fe.Runtime) (fe.Instance, error) {
	var cfg config
	if len(spec.Config) > 0 {
		if err := json.Unmarshal(spec.Config, &cfg); err != nil {
			return nil, fmt.Errorf("logger: decoding config: %w", err)
		}
	}
	if cfg.Level == "" {
		cfg.Level = "info" // default
	}
	return &Instance{level: cfg.Level}, nil
}

// Instance is a logger module instance. Its fields are runtime state only;
// config lives in the DTO above.
type Instance struct {
	level string
}

// Start implements fe.Instance.
func (i *Instance) Start() error {
	log.Printf("[logger] started (level=%s)", i.level)
	return nil
}

// Stop implements fe.Instance.
func (i *Instance) Stop() error {
	log.Printf("[logger] stopped")
	return nil
}

// Level returns the configured log level.
func (i *Instance) Level() string { return i.level }
