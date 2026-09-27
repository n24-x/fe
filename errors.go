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

	// ErrInstanceNotFound: Runtime.instance found no instance with the given
	// config.
	ErrInstanceNotFound = errors.New("fe: instance not found")

	// ErrAppStopped: App.Start was called after App.Stop. The App is
	// single-use — Stop is the process-exit path — and the Runtime that Start
	// had already built has been stopped again rather than installed.
	ErrAppStopped = errors.New("fe: app already stopped")
)
