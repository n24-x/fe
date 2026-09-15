package eventbus

import (
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/n24-x/fe/common/runtimex"
)

type Publisher[T any] struct {
	client *Client
	core   *publisherCore
}

type publisherCore struct {
	bus    *Bus
	client *Client
	router *router

	eventType reflect.Type
	once      sync.Once

	closed atomic.Bool
}

type publishedEvent struct {
	seq       uint64
	eventType reflect.Type
	event     any
}

func (p *Publisher[T]) Publish(event T) error { return p.core.publish(event) }

func (p *Publisher[T]) HasSubscriber() bool { return p.core.hasSubscriber() }

func (p *Publisher[T]) Close() { p.core.close() }

func (pc *publisherCore) publish(event any) error {
	if pc == nil || pc.client == nil {
		return ErrPublisherClosed
	}
	// follow Bus.Close() → Client.Close() ->
	// publisher.Close() -> pc.closed = true
	if pc.bus.closed.Load() {
		return ErrBusClosed
	}
	if pc.client.isClosed() {
		return ErrClientClosed
	}
	if pc.closed.Load() {
		return ErrPublisherClosed
	}

	ev := publishedEvent{eventType: pc.eventType, event: event}
	select {
	case pc.router.eventInput <- ev:
		return nil
	default:
	}

	// TODO issue #1
	if p := pc.bus.workerOfGID(runtimex.GoroutineID()); p != nil {
		return p.deferPublish(ev) // 延迟投递，回调不阻塞
	}

	select {
	case pc.router.eventInput <- ev:
		return nil
	case <-pc.router.done:
		return ErrBusClosed
	case <-pc.client.done:
		return ErrClientClosed
	}
}

func (pc *publisherCore) close() {
	if pc == nil {
		return
	}
	pc.once.Do(func() {
		pc.closed.Store(true)
		if pc.client == nil {
			return
		}
		pc.client.removePublisher(pc)
	})
}
