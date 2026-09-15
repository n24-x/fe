package eventbus

import (
	"fmt"
	"reflect"
)

func (c *Client) NewPublisher[T any]() (*Publisher[T], error) {
	core, err := c.registerPublisher(reflect.TypeFor[T]())
	if err != nil {
		return nil, err
	}
	return &Publisher[T]{client: c, core: core}, nil
}

func (c *Client) registerPublisher(eventType reflect.Type) (*publisherCore, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isClosed() {
		return nil, ErrClientClosed
	}

	if _, dup := c.pubs[eventType]; dup {
		return nil, fmt.Errorf("%w: client %q, type %s", ErrPublisherExists, c.name, eventType)
	}

	core := &publisherCore{client: c, bus: c.bus, router: c.bus.router, eventType: eventType}

	c.pubs[eventType] = core
	return core, nil
}

func (c *Client) removePublisher(core *publisherCore) {
	c.mu.Lock()
	if c.pubs[core.eventType] == core {
		delete(c.pubs, core.eventType)
	}
	c.mu.Unlock()
}
