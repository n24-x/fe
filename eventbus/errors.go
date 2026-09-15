package eventbus

import "errors"

var (
	ErrBusClosed        = errors.New("eventbus: bus is closed")
	ErrClientClosed     = errors.New("eventbus: client is closed")
	ErrPublisherClosed  = errors.New("eventbus: publisher is closed")
	ErrPublisherExists  = errors.New("eventbus: publisher already exists for this type")
	ErrSubscriberExists = errors.New("eventbus: subscriber already exists for this type")
)
