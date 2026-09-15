package eventbus

import "github.com/n24-x/fe/common/runtimex"

type clientWorker struct {
	client *Client
	bus    *Bus
	router *router

	deliveries        []*subscriberDelivery
	deliveriesVersion uint64

	heads map[*subscriberDelivery]publishedEvent
	kept  map[*subscriberDelivery]bool

	gid         uint64
	deferred    []publishedEvent
	deferredCap int

	done chan struct{}
}

func (w *clientWorker) start() {
	defer close(w.done)

	w.gid = runtimex.GoroutineID()
	w.bus.registerWorker(w)
	defer w.bus.unregisterWorker(w)
	c := w.client

	for {
		w.flushDeferred()

		select {
		case <-c.done:
			return
		default:
		}

		// 1.
		w.sync()

		// 2.
		for _, d := range w.deliveries {
			if _, ok := w.heads[d]; ok {
				continue
			}
			if ev, ok := w.pull(d); ok {
				w.heads[d] = ev
			}
		}

		// 3. 无事件可投递：等待唤醒或关闭。
		if len(w.heads) == 0 {
			select {
			case <-c.done:
				return
			case <-c.workerWake:
				continue
			}
		}

		// 4. 挑出 seq 最小者并投递（跨类型全局顺序，小顶堆语义）。
		w.push()
	}
}

func (w *clientWorker) sync() {
	v := w.client.subsVersion.Load()
	if v == w.deliveriesVersion {
		return
	}
	c := w.client
	c.mu.Lock()
	w.deliveries = w.deliveries[:0]
	for _, core := range c.subs {
		w.deliveries = append(w.deliveries, core.delivery)
	}
	c.mu.Unlock()
	if w.kept == nil {
		w.kept = make(map[*subscriberDelivery]bool, len(w.deliveries))
	} else {
		clear(w.kept)
	}
	for _, d := range w.deliveries {
		w.kept[d] = true
	}
	for d := range w.heads {
		if !w.kept[d] {
			delete(w.heads, d)
		}
	}
	w.deliveriesVersion = v
}

func (w *clientWorker) push() {
	var nextSD *subscriberDelivery
	for sd, ev := range w.heads {
		if nextSD == nil || ev.seq < w.heads[nextSD].seq {
			nextSD = sd
		}
	}
	ev := w.heads[nextSD]
	delete(w.heads, nextSD)
	select {
	case <-nextSD.done:
	default:
		nextSD.deliverFunc(ev)
	}
}

func (w *clientWorker) pull(sd *subscriberDelivery) (publishedEvent, bool) {
	if sd.policy != OverflowDropOldest {
		select {
		case ev := <-sd.inbox:
			return ev, true
		default:
			return publishedEvent{}, false
		}
	}

	// DropOldest
	select {
	case ev := <-sd.inbox: // ev will be dropped
		for {
			select {
			case e := <-sd.inbox:
				sd.drop(1)
				ev = e
			default:
				return ev, true
			}
		}
	default:
		return publishedEvent{}, false
	}
}

func (w *clientWorker) deferPublish(ev publishedEvent) error {
	if len(w.deferred) >= w.deferredCap {
		select {
		case w.router.eventInput <- ev:
			return nil
		case <-w.router.done:
			return ErrBusClosed
		case <-w.client.done:
			return ErrClientClosed
		}
	}
	w.deferred = append(w.deferred, ev)
	return nil
}

func (w *clientWorker) flushDeferred() {
	if len(w.deferred) == 0 {
		return
	}

	eventInput := w.router.eventInput
	for len(w.deferred) > 0 {
		select {
		case eventInput <- w.deferred[0]:
			w.deferred = w.deferred[1:]
		default:
			return
		}
	}

	w.deferred = nil
}
