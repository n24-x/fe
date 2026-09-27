package feconfig

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"uuid"

	dag "github.com/n24-x/dag-go"
	"github.com/n24-x/fe/eventbus"
)

// MachineConfig is the configuration the framework consumes: the Module
// Instances to create and run, plus the framework's own facility options.
//
// It is a plain in-process value, and this type is the contract: the field
// names and json tags below are the shape the framework reads. Decoding is not
// feconfig's job — how a config is written, generated, or parsed is the
// application's business, and the framework exports no parser.
//
// MachineConfigValidate checks the structure of a decoded value; the framework
// adds its own checks when it builds a Runtime.
type MachineConfig struct {
	Options   Options        `json:"options"`
	Instances []InstanceSpec `json:"instances"`
}

// Options configures Runtime facilities provided by the framework.
type Options struct {
	Bus eventbus.BusOptions `json:"bus"`

	// Logger is injected by the framework; see [fe.App.Start].
	// json:"-" keeps it out of config decoding.
	Logger *slog.Logger `json:"-"`
}

// InstanceSpec describes one Module Instance to create and run.
type InstanceSpec struct {
	// InstanceID is this instance's identity: a v4 uuid, unique in the config.
	// Modules reference other instances by it (see Deps).
	InstanceID string `json:"id"`

	// ModuleID names the registered module that produces the instance.
	ModuleID string `json:"mod_id"`

	// Config is the module's own configuration; the framework does not decode
	// it, it hands it to the module untouched.
	Config json.RawMessage `json:"config"`

	// Deps are the ids of the other instances in this config that this one
	// depends on. The framework derives the order from them, and provisions,
	// starts and stops in that order.
	Deps []string `json:"deps"`
}

// MachineConfigValidate performs semantic validation of an already-decoded
// config: every mod_id is non-empty, every id is a v4 uuid, ids are unique,
// deps reference declared instances, and the dependency graph is acyclic.
//
// It is a pure check on a value: it never decodes anything and has no side
// effects.
func MachineConfigValidate(mc *MachineConfig) error {
	if mc == nil {
		return fmt.Errorf("%w: nil machine config", ErrMalformedConfig)
	}

	ids := make(map[string]struct{}, len(mc.Instances))
	for i, inst := range mc.Instances {
		if inst.ModuleID == "" {
			return fmt.Errorf("%w: instances[%d].mod_id: must not be empty", ErrMissingModuleID, i)
		}
		u, err := uuid.Parse(inst.InstanceID)
		if err != nil {
			return fmt.Errorf("%w: instances[%d].id %q: %w", ErrInvalidID, i, inst.InstanceID, err)
		}

		if u[6]>>4 != 4 {
			return fmt.Errorf("%w: instances[%d].id %q: not a v4 uuid", ErrInvalidID, i, inst.InstanceID)
		}
		if _, ok := ids[inst.InstanceID]; ok {
			return fmt.Errorf("%w: instances[%d].id %q", ErrDuplicateID, i, inst.InstanceID)
		}
		ids[inst.InstanceID] = struct{}{}
	}

	for i, inst := range mc.Instances {
		for j, dep := range inst.Deps {
			if _, ok := ids[dep]; !ok {
				return fmt.Errorf("%w: instances[%d].deps[%d]: %q not defined in this config", ErrUndefinedDep, i, j, dep)
			}
		}
	}

	// Build a dependency graph and reject cyclic configs.
	g := dag.New[string]()
	for i := range mc.Instances {
		// &mc.Instances[i] is each element's own address: the graph nodes
		// must be distinct (and alias the config's specs, so g.At returns
		// the original InstanceSpec).
		if _, _, err := g.Add(&mc.Instances[i]); err != nil {
			// TODO: dag.CycleError.Error() prints numeric NodeIDs (e.g. "[2 3 2]"),
			// unreadable here. The cycle's nodes are available via e.Nodes for a
			// future uuid-based message.
			return fmt.Errorf("%w: dag.Graph %w", ErrCyclicDeps, err)
		}
	}

	return nil
}

// Requires implements dag.Node[string]: the instance depends on its deps' ids.
func (inst *InstanceSpec) Requires() []string {
	return inst.Deps
}

// Provides implements dag.Node[string]: the instance provides its own id.
func (inst *InstanceSpec) Provides() []string {
	return []string{inst.InstanceID}
}

var _ dag.Node[string] = (*InstanceSpec)(nil)
