package eventbus

// hasSubscriber 报告总线上是否已有类型 eventType 的订阅方。
func (pc *publisherCore) hasSubscriber() bool {
	if pc == nil || pc.client == nil {
		return false
	}
	return pc.bus.hasSubscriber(pc.eventType)
}
