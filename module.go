package fe

import (
	"context"
	"fmt"
	"sync"

	"github.com/n24-x/fe/feconfig"
)

// ModuleID is a namespaced, dot-separated module identifier,
// e.g. "dns.resolver" or "endpoint.proxy.server".
type ModuleID string

// Module is a module type, registered via [RegisterModule] (typically in init,
// as an import side effect).
//
// The framework instantiates a Module only if it also implements
// [Provisioner]; otherwise it is a registry citizen that config CAN NOT
// reference as an instance.
type Module interface {
	// FeModule describes the module. This method MUST NOT have any side-effects.
	FeModule() ModuleInfo
}

// ModuleInfo describes a registered module.
type ModuleInfo struct {
	// ID is the full name of the module. It MUST be unique.
	ID ModuleID
}

// Package-level module registry.
// Modules are registered at init time and never change afterwards, so one
// process has exactly one registry, shared by all Runtimes. The registry
// stores the stateless Module value itself (identity + the receiver for
// Provision).
var (
	modules   = make(map[ModuleID]Module)
	modulesMu sync.RWMutex
)

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

// RegisterModule registers a module. It should be called during init.
// It panics if m is nil, its ID is missing, or the module is already
// registered.
func RegisterModule(m Module) {
	if m == nil {
		panic("fe: RegisterModule: nil module")
	}

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

func (mi ModuleInfo) String() string {
	return string(mi.ID)
}

// RuntimeAccess is the module-visible view of the Runtime handed to a
// Provision call: resolve a dependency instance, read the lifecycle context.
// It deliberately hides the rest of the Runtime (lifecycle, registry, …).
// *Runtime satisfies it; tests may fake it.
type RuntimeAccess interface {
	// Instance resolves a dependency instance by its config id.
	Instance(id string) (Instance, error)
	// Context returns the Runtime's lifecycle context, read-only.
	Context() context.Context
}

// Provisioner is implemented by Modules that produce Instances. The module
// author:
//   - parses spec.Config (the framework does not decode it);
//   - captures runtime needs — dependencies via rt.Instance, the lifecycle
//     context via rt.Context — into the Instance's fields;
//   - returns a fresh, self-contained Instance.
type Provisioner interface {
	Provision(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error)
}
