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
	// config.
	ErrInstanceNotFound = errors.New("fe: instance not found")

	// ErrManagerStopped: Manager.Apply was called after Manager.Stop. The
	// Manager is single-use — Stop is the process-exit path — and the Runtime
	// that Apply had already built has been stopped again rather than
	// installed.
	ErrManagerStopped = errors.New("fe: manager already stopped")
)
