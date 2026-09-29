package fe

import "uuid"

// InstanceID is the ID of a Module Instance, stored as UUIDv4.
// It is assigned by the config: see [feconfig.InstanceSpec].
type InstanceID uuid.UUID

// String returns the canonical UUID form, e.g.
// "9b2e7d1c-3f4a-4b5c-8d6e-7f8a9b0c1d2e".
//
// InstanceID is a defined type rather than an alias, so it does not inherit
// uuid.UUID's methods — without this one the framework's own error messages
// render the id as raw bytes ("%s") or a byte slice ("%v"), which is exactly
// the wrong thing to hand someone debugging a failed Start.
func (id InstanceID) String() string { return uuid.UUID(id).String() }

// Instance is one running instance of a Module.
//
// It has a parameterless lifecycle: everything an instance needs at runtime,
// including its identity, dependency references, and lifecycle signal, is
// captured in its fields during Provision.
//
// If Start returns an error, the instance MUST be left stopped and require no
// cleanup through Stop: Stop is called only after a successful Start. Once
// Start succeeds, Stop is responsible for releasing all resources acquired by
// the instance. A Start that panics falls under the same rule, and the instance
// releases what it acquired in its own defer: the framework never calls Stop
// for it.
//
// A panic in a goroutine the instance started is the module author's to handle:
// the framework does not own that goroutine and releases nothing for it.
//
// Start and Stop have no timeout. They may block for as long as the module
// needs, and the framework does not bound, interrupt, or otherwise monitor
// them. Keeping them prompt is the module author's job; what to do about an
// instance that never returns is the application's.
//
// The framework drives the lifecycle: Runtime.Start starts instances in
// dependency order, and Runtime.Stop stops them in reverse order. Framework
// concepts such as identity, state, and dependencies are not part of this
// interface.
type Instance interface {
	Start() error
	Stop() error
}
