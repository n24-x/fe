package eventbus

import (
	"reflect"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/n24-x/fe/common/runtimex"
)

type Bus struct {
	router *router

	topicsMu sync.RWMutex
	topics   map[reflect.Type][]*subscriberDelivery

	clientsMu sync.Mutex
	clients   []*Client

	workersMu sync.Mutex
	workers   map[uint64]*clientWorker

	closed atomic.Bool

	counters busCounters
}

// BusOptions configures a Bus.
type BusOptions struct {
	// RouterCapacity is the capacity of the router's event input channel.
	RouterCapacity int `json:"router_capacity"`
}

type busCounters struct {
	published atomic.Uint64
	routed    atomic.Uint64
	dropped   atomic.Uint64
}

func New() *Bus { return NewWithOptions(BusOptions{}) }

func NewWithOptions(opts BusOptions) *Bus {
	capacity := opts.RouterCapacity
	if capacity <= 0 {
		capacity = DefaultRouterCapacity
	}

	b := &Bus{
		topics:  map[reflect.Type][]*subscriberDelivery{},
		workers: map[uint64]*clientWorker{},
	}

	b.router = &router{
		bus:        b,
		eventInput: make(chan publishedEvent, capacity),
		done:       make(chan struct{}),
		exited:     make(chan struct{}),
	}

	go b.router.start()
	return b
}

func (b *Bus) NewClient(name string) (*Client, error) {
	b.clientsMu.Lock()
	defer b.clientsMu.Unlock()
	if b.closed.Load() {
		return nil, ErrBusClosed
	}

	c := &Client{
		bus:        b,
		name:       name,
		subs:       map[reflect.Type]*subscriberCore{},
		pubs:       map[reflect.Type]*publisherCore{},
		done:       make(chan struct{}),
		workerWake: make(chan struct{}, 1),
	}
	b.clients = append(b.clients, c)
	return c, nil
}

func (b *Bus) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}

	b.router.close()

	b.clientsMu.Lock()
	clients := b.clients
	b.clients = nil
	b.clientsMu.Unlock()
	for _, c := range clients {
		c.Close()
	}
}

// Hotpath
func (b *Bus) deliverySnapshot(eventType reflect.Type) []*subscriberDelivery {
	b.topicsMu.RLock()
	defer b.topicsMu.RUnlock()
	return b.topics[eventType]
}

func (b *Bus) removeClient(client *Client) {
	b.clientsMu.Lock()
	defer b.clientsMu.Unlock()
	if i := slices.Index(b.clients, client); i >= 0 {
		b.clients = slices.Delete(b.clients, i, i+1)
	}
}

func (b *Bus) addTopicSubscriber(eventType reflect.Type, sd *subscriberDelivery) {
	b.topicsMu.Lock()
	defer b.topicsMu.Unlock()
	oldSDs := b.topics[eventType]
	newSDs := make([]*subscriberDelivery, len(oldSDs)+1)
	copy(newSDs, oldSDs)
	newSDs[len(oldSDs)] = sd
	b.topics[eventType] = newSDs
}

// Note 发出去的快照保持不变
func (b *Bus) removeTopicSubscriber(eventType reflect.Type, sd *subscriberDelivery) {
	b.topicsMu.Lock()
	defer b.topicsMu.Unlock()
	old := b.topics[eventType]
	i := slices.Index(old, sd)
	if i < 0 {
		return // sd 不在链上：静默无操作（幂等）
	}
	if len(old) == 1 {
		delete(b.topics, eventType)
		return
	}
	b.topics[eventType] = slices.Concat(old[:i], old[i+1:])
}

func (b *Bus) registerWorker(w *clientWorker) {
	b.workersMu.Lock()
	defer b.workersMu.Unlock()
	b.workers[w.gid] = w
}

func (b *Bus) unregisterWorker(w *clientWorker) {
	b.workersMu.Lock()
	defer b.workersMu.Unlock()
	delete(b.workers, w.gid)
}

func (b *Bus) workerOfGID(gid uint64) *clientWorker {
	b.workersMu.Lock()
	w := b.workers[gid]
	b.workersMu.Unlock()
	return w
}

func (b *Bus) onWorker(w *clientWorker) bool {
	return w != nil && b.workerOfGID(runtimex.GoroutineID()) == w
}
