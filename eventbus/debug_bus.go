package eventbus

import "reflect"

// BusStats is a snapshot of cumulative bus statistics.
type BusStats struct {
	PublishedTotal uint64 // Total events published.
	RoutedTotal    uint64 // Total successful [router.submit] calls (not deliveries; see TODO in [busCounters]).
	DroppedTotal   uint64 // Total dropped due to overflow.
	TopicCount     int    // Number of event types with subscribers.
	ClientCount    int    // Number of live clients.
}

func (b *Bus) Stats() BusStats {
	b.clientsMu.Lock()
	cc := len(b.clients)
	b.clientsMu.Unlock()

	b.topicsMu.RLock()
	tc := len(b.topics)
	b.topicsMu.RUnlock()
	return BusStats{
		PublishedTotal: b.counters.published.Load(),
		RoutedTotal:    b.counters.routed.Load(),
		DroppedTotal:   b.counters.dropped.Load(),
		TopicCount:     tc,
		ClientCount:    cc,
	}
}

func (b *Bus) countPublished()       { b.counters.published.Add(1) }
func (b *Bus) countRouted()          { b.counters.routed.Add(1) }
func (b *Bus) countDropped(n uint64) { b.counters.dropped.Add(n) }

// hasSubscriber reports whether eventType currently has at least one subscriber.
// Publishers may use this to skip expensive event construction.
func (b *Bus) hasSubscriber(eventType reflect.Type) bool {
	b.topicsMu.RLock()
	defer b.topicsMu.RUnlock()
	return len(b.topics[eventType]) > 0
}
