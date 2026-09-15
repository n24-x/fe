package eventbus

const (
	DefaultRouterCapacity     = 128
	DefaultSubscriberCapacity = 16
)

type OverflowPolicy uint8

const (
	OverflowBlock OverflowPolicy = iota
	OverflowDropNewest
	OverflowDropOldest
)
