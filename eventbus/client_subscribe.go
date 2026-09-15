package eventbus

import (
	"fmt"
	"reflect"
)

func (c *Client) Subscribe[T any](opts SubscribeOptions) (*Subscriber[T], error) {
	eventType := reflect.TypeFor[T]()
	core := newSubscriberCore(c, eventType, opts)
	sub := &Subscriber[T]{core: core, events: make(chan T)}
	core.delivery.deliverFunc = func(ev publishedEvent) {
		v := ev.event.(T)
		select {
		case sub.events <- v:
		case <-core.done:
		}
	}
	if err := c.addSubscriber(core); err != nil {
		return nil, err
	}
	return sub, nil
}

func (c *Client) SubscribeFunc[T any](fn func(T), opts SubscribeOptions) (*SubscriberFunc[T], error) {
	eventType := reflect.TypeFor[T]()

	core := newSubscriberCore(c, eventType, opts)
	core.delivery.deliverFunc = func(ev publishedEvent) {
		// TODO(next) A panicking subscriber callback must not take down the worker
		// goroutine, so the panic is recovered and dropped here.
		defer func() { _ = recover() }()
		fn(ev.event.(T))
	}

	if err := c.addSubscriber(core); err != nil {
		return nil, err
	}
	return &SubscriberFunc[T]{core: core}, nil
}

func (c *Client) addSubscriber(core *subscriberCore) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isClosed() {
		return ErrClientClosed
	}

	if _, dup := c.subs[core.eventType]; dup {
		return fmt.Errorf("%w: client %q, type %s", ErrSubscriberExists, c.name, core.eventType)
	}

	c.subs[core.eventType] = core
	c.bus.addTopicSubscriber(core.eventType, core.delivery) // 此时 订阅对 router 可见
	c.subsVersion.Add(1)
	c.startWorker() // 首个订阅时懒启动 worker goroutine（持 c.mu），后续无效
	c.wakeWorker()  // client.subsVersion 发生了变化，这里需要立刻唤醒 worker 同步信息
	return nil
}

func (c *Client) removeSubscriber(core *subscriberCore) {
	c.mu.Lock()
	if c.subs[core.eventType] == core {
		delete(c.subs, core.eventType)
	}
	c.subsVersion.Add(1)
	c.mu.Unlock()

	c.bus.removeTopicSubscriber(core.eventType, core.delivery)
	close(core.delivery.done)
	c.wakeWorker() // client.subsVersion 发生了变化，这里需要立刻唤醒 worker 同步信息
}
