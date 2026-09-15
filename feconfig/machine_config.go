package feconfig

import (
	"encoding/json"
	"fmt"
	"uuid"

	dag "github.com/n24-x/dag-go"
	"github.com/n24-x/fe/eventbus"
)

// MachineConfig is the configuration consumed by fe: the list of Module
// Instances to create and run, plus the framework's own facility options.
//
// It is a plain in-process value. It is produced either by an adapter that
// translates a human-readable config, or by parsing its JSON representation
// (see [ParseHelper]).
//
// On the global section: an earlier revision carried shared instance
// configuration here and it was removed — modules are third-party and the
// framework cannot know what they would share; instances that need shared
// config depend on a shared instance instead. Options below is a different
// thing: it configures fe's own facilities, and is consumed by the framework
// rather than by module authors.
type MachineConfig struct {
	Options   Options        `json:"options"`
	Instances []InstanceSpec `json:"instances"`
}

// Options is framework-facility configuration: settings for the Runtime's own
// facilities (currently the event bus), consumed by fe itself.
type Options struct {
	Bus eventbus.BusOptions `json:"bus"`
}

type InstanceSpec struct {
	InstanceID string          `json:"id"`
	ModuleID   string          `json:"mod_id"`
	Config     json.RawMessage `json:"config"`
	Deps       []string        `json:"deps"`
}

// MachineConfigValidate performs semantic validation of an already-decoded
// config: every mod_id is non-empty, every id is a v4 uuid, ids are unique,
// deps reference declared instances, and the dependency graph is acyclic.
//
// It is a pure check on a value — it never decodes anything (parsing is
// [ParseHelper]'s job) and has no side effects.
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
			return fmt.Errorf("%w: instances[%d].id %q: %v", ErrInvalidID, i, inst.InstanceID, err)
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
			return fmt.Errorf("%w: dag.Graph %v", ErrCyclicDeps, err)
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
