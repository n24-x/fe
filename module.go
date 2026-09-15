package fe

import (
	"cmp"
	"fmt"
	"slices"
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

// Modules returns the descriptor of every registered module, sorted by ID.
//
// The registry is a map, so the sort is what makes the result useful: a
// caller that displays it — CLI help, or an adapter suggesting a module name
// for a misspelled mod_id — would otherwise print a different order on every
// run.
//
// It returns a fresh snapshot on each call; mutating the slice does not touch
// the registry.
func Modules() []ModuleInfo {
	modulesMu.RLock()
	mods := make([]ModuleInfo, 0, len(modules))
	for _, m := range modules {
		mods = append(mods, m.FeModule())
	}
	modulesMu.RUnlock()

	// Sorted outside the lock: the snapshot is ours now.
	slices.SortFunc(mods, func(a, b ModuleInfo) int { return cmp.Compare(a.ID, b.ID) })
	return mods
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

// Provisioner is implemented by Modules that produce Instances. The module
// author:
//   - parses spec.Config (the framework does not decode it);
//   - captures runtime needs — dependencies via rt.Instance, the lifecycle
//     signal via rt.Done — into the Instance's fields;
//   - returns a fresh, self-contained Instance.
type Provisioner interface {
	Provision(spec feconfig.InstanceSpec, rt RuntimeAccess) (Instance, error)
}
