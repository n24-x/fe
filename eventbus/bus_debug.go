package eventbus

import "reflect"

// BusStats 是总线累计统计的快照。
type BusStats struct {
	PublishedTotal uint64 // 累计发布事件数
	RoutedTotal    uint64 // 累计递交份数（router.submit 成功一次计 1；非"送达"数，见 busCounters 的 TODO）
	DroppedTotal   uint64 // 累计因溢出丢弃数
	TopicCount     int    // 当前有订阅方的事件类型数
	ClientCount    int    // 当前存活 Client 数
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

// hasSubscriber 报告类型 eventType 当前是否至少有一个订阅方。
// 发布方可借此跳过昂贵的事件构造；路由以 deliverySnapshot 的实际快照为准。
func (b *Bus) hasSubscriber(eventType reflect.Type) bool {
	b.topicsMu.RLock()
	defer b.topicsMu.RUnlock()
	return len(b.topics[eventType]) > 0
}
