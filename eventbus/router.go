package eventbus

type router struct {
	bus *Bus

	eventInput chan publishedEvent

	done   chan struct{}
	exited chan struct{}
}

func (r *router) start() {
	defer close(r.exited) // Mark the router as exited
	var seq uint64
	for {
		select {
		case <-r.done:
			return
		default:
		}

		select {
		case <-r.done:
			return
		case ev := <-r.eventInput:
			ev.seq = seq
			seq++
			r.bus.countPublished()
			r.route(ev)
		}
	}
}

func (r *router) close() {
	close(r.done)
	<-r.exited // Wait until the router is exited
}

func (r *router) route(ev publishedEvent) {
	for _, sd := range r.bus.deliverySnapshot(ev.eventType) {
		select {
		case <-r.done:
			return
		default:
		}
		if r.submit(sd, ev) {
			r.bus.countRouted()
		}
	}
}

func (r *router) submit(sd *subscriberDelivery, ev publishedEvent) bool {
	switch sd.policy {
	case OverflowBlock, OverflowDropOldest:
		return r.submitBlocking(sd, ev)
	case OverflowDropNewest:
		select {
		case sd.inbox <- ev:
			sd.client.wakeWorker()
			return true
		default:
			sd.drop(1) // drop newest event
			return false
		}
	}
	return false
}

func (r *router) submitBlocking(sd *subscriberDelivery, ev publishedEvent) bool {
	select {
	case sd.inbox <- ev:
		sd.client.wakeWorker()
		return true
	case <-sd.done:
		return false
	case <-r.done:
		return false
	}
}
