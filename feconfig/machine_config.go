package feconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"uuid"

	dag "github.com/n24-x/dag-go"
)

// MachineConfig is the machine-readable configuration consumed by fe: the
// complete list of Module Instances to create and run. It is typically
// generated from a human-readable config by an adapter.
//
// TODO(next)
// It deliberately has no global section: config shared by several instances
// is expressed by those instances depending on a shared instance (see
// issue.md D29).
type MachineConfig struct {
	Instances []InstanceSpec `json:"instances"`
}

type InstanceSpec struct {
	InstanceID string          `json:"id"`
	ModuleID   string          `json:"mod_id"`
	Config     json.RawMessage `json:"config"`
	Deps       []string        `json:"deps"`
}

func MachineConfigValidate(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var mc MachineConfig
	if err := dec.Decode(&mc); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedConfig, err)
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
