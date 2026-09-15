package fe

import "uuid"

// InstanceID is the ID of a Module Instance, stored as UUIDv4.
// It is assigned by the config: see [feconfig.InstanceSpec].
type InstanceID uuid.UUID

// Instance is one running instance of a Module.
//
// It is a pure lifecycle with no parameters: everything an instance needs at
// runtime (identity, dependency references, the lifecycle signal) is captured
// into its fields during Provision.
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
