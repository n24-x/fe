package fe

import (
	"fmt"
	"strings"
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// ModuleID is a namespaced, dot-separated module identifier,
// e.g. "dns.resolver" or "endpoint.proxy.server".
type ModuleID string

// Module is a module type. Modules register themselves via RegisterModule,
// typically during init (as an import side effect).
//
// A Module is stateless and shared: it only carries identity. Per-config
// state lives in the Instance each Provision call returns, never on the
// registered Module value itself.
type Module interface {
	// FeModule describes the module. This method MUST not have any side-effects.
	FeModule() ModuleInfo
}

// ModuleInfo describes a registered module.
type ModuleInfo struct {
	// ID is the full name of the module. It must be unique.
	ID ModuleID
}

func (mi ModuleInfo) String() string {
	return string(mi.ID)
}

// Provisioner is implemented by Modules that produce Instances. When a
// MachineConfig references such a module, the Runtime calls Provision once
// per instance spec to build that instance.
//
// Provision is a factory: it MUST return a fresh, self-contained Instance
// with no shared state on the receiver. The spec carries the instance's
// config (parsing is the module author's job — the framework does not decode
// it), and rt provides runtime access (dependencies via rt.Instance, the
// lifecycle context via rt.Context).
//
// Modules that do NOT implement Provisioner are registry citizens but can
// never be referenced by a config as an instance; ValidateRuntimeConfig
// rejects such references.
type Provisioner interface {
	Provision(spec feconfig.InstanceSpec, rt *Runtime) (Instance, error)
}

// Namespace returns the namespace (all but the last label) of a module ID.
// An ID with no dot has an empty namespace.
func (id ModuleID) Namespace() string {
	lastDot := strings.LastIndex(string(id), ".")
	if lastDot < 0 {
		return ""
	}
	return string(id)[:lastDot]
}

// Name returns the last label of a module ID.
func (id ModuleID) Name() string {
	s := string(id)
	pos := strings.LastIndex(s, ".")
	if pos == -1 {
		return s
	}
	return s[pos+1:]
}

// Package-level module registry.
// Modules are registered at init time and never change afterwards, so one
// process has exactly one registry, shared by all Runtimes (see D14).
// The registry stores the stateless Module value itself (identity + the
// receiver for Provision); it is NOT a per-Runtime structure.
var (
	modules   = make(map[ModuleID]Module)
	modulesMu sync.RWMutex
)

// RegisterModule registers a module. It should be called during init.
// It panics if the module ID is missing or the module is already registered.
// Implementing Provisioner is NOT enforced here: modules without instances
// may still register (they just can never be referenced by a config).
func RegisterModule(m Module) {
	mi := m.FeModule()

	if mi.ID == "" {
		panic("fe: module ID missing")
	}

	modulesMu.Lock()
	defer modulesMu.Unlock()
	if _, ok := modules[mi.ID]; ok {
		panic(fmt.Sprintf("fe: module already registered: %s", mi.ID))
	}
	modules[mi.ID] = m
}

// GetModule returns the registered module value by its ID.
func GetModule(id ModuleID) (Module, error) {
	modulesMu.RLock()
	defer modulesMu.RUnlock()

	m, ok := modules[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrModuleNotRegistered, id)
	}
	return m, nil
}
