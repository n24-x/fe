package eventbus

import (
	"reflect"
	"sync"
	"sync/atomic"
)

type Client struct {
	bus  *Bus
	name string

	mu          sync.Mutex
	subs        map[reflect.Type]*subscriberCore
	subsVersion atomic.Uint64
	pubs        map[reflect.Type]*publisherCore

	worker *clientWorker
	// 非阻塞：令牌通道缓冲 1 自动去重——worker 忙时会自行继续拉取，无需多次唤醒。
	workerWake chan struct{}

	done chan struct{}
}

func (c *Client) Close() {
	c.mu.Lock()
	if c.isClosed() {
		c.mu.Unlock()
		return
	}

	close(c.done)

	subs := c.subs
	c.subs = nil

	pubs := c.pubs
	c.pubs = nil

	w := c.worker
	c.mu.Unlock()

	c.bus.removeClient(c)

	for _, core := range subs {
		core.close()
	}

	for _, core := range pubs {
		core.close()
	}

	if w != nil && !c.bus.onWorker(w) {
		<-w.done
	}
}

func (c *Client) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Client) Done() <-chan struct{} { return c.done }

func (c *Client) startWorker() {
	if c.worker != nil {
		return
	}
	w := &clientWorker{
		client:      c,
		bus:         c.bus,
		router:      c.bus.router,
		done:        make(chan struct{}),
		heads:       make(map[*subscriberDelivery]publishedEvent),
		deferredCap: cap(c.bus.router.eventInput),
	}
	c.worker = w
	go w.start()
}

func (c *Client) wakeWorker() {
	if c == nil || c.workerWake == nil {
		return
	}
	select {
	case c.workerWake <- struct{}{}:
	default:
	}
}
