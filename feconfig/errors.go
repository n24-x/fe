package feconfig

import "errors"

var (
	// ErrMalformedConfig: the value is not a usable machine config (nil, or a shape that cannot describe instances).
	// Decoding is not feconfig's job — the framework takes a MachineConfig however the application built it —
	// so this is never returned for a JSON syntax error.
	ErrMalformedConfig = errors.New("feconfig: malformed machine config")

	// ErrMissingModuleID: instances[i].mod_id is empty.
	// Whether mod_id is registered is the fe (Runtime) validation's job, not this layer's.
	ErrMissingModuleID = errors.New("feconfig: missing mod_id")

	// ErrInvalidID: instances[i].id is empty, not a valid UUID, or not v4.
	ErrInvalidID = errors.New("feconfig: invalid instance id")

	// ErrUndefinedDep: instances[i].deps[j] references an instance id not defined in this config.
	ErrUndefinedDep = errors.New("feconfig: dep references undefined instance")

	// ErrDuplicateID: instances contains a duplicate instance id.
	ErrDuplicateID = errors.New("feconfig: duplicate instance id")

	// ErrCyclicDeps: instances' deps form a cycle (dag.CycleError).
	ErrCyclicDeps = errors.New("feconfig: cyclic instance deps")
)
