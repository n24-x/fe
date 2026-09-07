package fe

import "uuid"

// InstanceID is the ID of a Module Instance, stored as UUIDv4.
// Instance identity comes from the config's "id" field (see D9), not
// from the Instance itself.
type InstanceID uuid.UUID

// Instance is one running instance of a Module (caddy's App). It is a pure
// lifecycle with no parameters: everything an instance needs at runtime
// (identity, dependency references, the lifecycle context) is captured into
// its fields during Provision, so Start/Stop only read fields and act.
//
// The framework drives the lifecycle (issue.md D21): Runtime.Start runs
// instances in creation order (deps first), Runtime.Stop in reverse.
// Framework concepts (ID/state/deps) are deliberately NOT part of this
// interface (issue.md §8.5).
type Instance interface {
	Start() error
	Stop() error
}
