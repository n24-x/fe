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
// It is a pure lifecycle with no parameters: everything an instance needs at
// runtime (identity, dependency references, the lifecycle signal) is captured
// into its fields during Provision.
//
// Neither method has a timeout: Start and Stop may block for as long as the
// module needs, and the framework neither bounds them nor watches them
// (issue.md D37). Controlling duration is the module's own job — through a
// goroutine it starts and its Stop reaps, a deadline on the waits it
// controls, or whatever fits. The framework's answer to an instance that
// never returns is the application's: a second interrupt, not a deadline
// (issue.md D31).
//
// TODO(next):
// The framework drives the lifecycle (issue.md D21): Runtime.Start runs
// instances in creation order (deps first), Runtime.Stop in reverse.
// Framework concepts (ID/state/deps) are deliberately NOT part of this
// interface (issue.md §8.5).
type Instance interface {
	Start() error
	Stop() error
}
