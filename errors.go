package fe

import "errors"

// Sentinel errors returned by fe functions. Test with errors.Is.
var (
	// ErrModuleNotRegistered: the module ID is not in the global registry.
	ErrModuleNotRegistered = errors.New("fe: module not registered")

	// ErrModuleNotProvisioner: the module is registered but does not
	// implement Provisioner, so it cannot be referenced by a config as an
	// instance.
	ErrModuleNotProvisioner = errors.New("fe: module does not produce instances (missing Provisioner)")

	// ErrInstanceNotFound: Runtime.Instance found no instance with the given
	// config id. A well-formed id that is missing usually means the
	// referenced instance was never declared, or was referenced in a config
	// but omitted from the referencing instance's deps (D11).
	ErrInstanceNotFound = errors.New("fe: instance not found")
)
