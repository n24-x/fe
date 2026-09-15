package eventbus

import (
	"reflect"
	"sync"
	"sync/atomic"
)

type Subscriber[T any] struct {
	core   *subscriberCore
	events chan T
}

type SubscriberFunc[T any] struct {
	core *subscriberCore
}

type SubscribeOptions struct {
	Capacity int
	Overflow OverflowPolicy
}

type subscriberCore struct {
	eventType  reflect.Type
	delivery   *subscriberDelivery
	done       chan struct{}
	unregister func(reflect.Type)
	once       sync.Once
	closed     atomic.Bool
}

type subscriberDelivery struct {
	client *Client
	policy OverflowPolicy

	inbox       chan publishedEvent
	deliverFunc func(publishedEvent)
	dropped     atomic.Uint64

	done chan struct{}
}

func newSubscriberCore(client *Client, eventType reflect.Type, opts SubscribeOptions) *subscriberCore {
	core := &subscriberCore{
		eventType: eventType,
		done:      make(chan struct{}),
	}

	core.delivery = newSubscriberDelivery(client, opts)
	core.unregister = func(t reflect.Type) { client.removeSubscriber(core) }
	return core
}

func newSubscriberDelivery(client *Client, opts SubscribeOptions) *subscriberDelivery {
	capacity := opts.Capacity
	if capacity <= 0 {
		capacity = DefaultSubscriberCapacity
	}
	inboxCap := capacity
	if opts.Overflow != OverflowDropNewest {
		inboxCap = 2 * capacity
	}
	return &subscriberDelivery{
		client: client,
		policy: opts.Overflow,
		inbox:  make(chan publishedEvent, inboxCap),
		done:   make(chan struct{}),
	}
}

func (s *Subscriber[T]) Events() <-chan T      { return s.events }
func (s *Subscriber[T]) Done() <-chan struct{} { return s.core.done }
func (s *Subscriber[T]) Close()                { s.core.close() }

func (s *SubscriberFunc[T]) Done() <-chan struct{} { return s.core.done }
func (s *SubscriberFunc[T]) Close()                { s.core.close() }

func (sd *subscriberDelivery) drop(n uint64) {
	if sd == nil {
		return
	}
	sd.dropped.Add(n)
	if sd.client != nil {
		sd.client.bus.countDropped(n)
	}
}

func (sc *subscriberCore) close() {
	if sc == nil {
		return
	}
	sc.once.Do(func() {
		sc.closed.Store(true)
		if sc.unregister != nil {
			sc.unregister(sc.eventType)
		}
		if sc.done != nil {
			close(sc.done)
		}
	})
}
