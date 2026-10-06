package directivefile

import "errors"

var (
	ErrAdapterNotRegistered = errors.New("directivefile: adapter not registered")
	ErrNoImplicitInstance   = errors.New("directivefile: no implicit instance registered for module")
	ErrInstanceNotFound     = errors.New("directivefile: instance not found")
)
