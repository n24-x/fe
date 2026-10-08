# EventBus

The EventBus is the in-process publish/subscribe channel between instances. One
Bus belongs to each Runtime, and **an event's Go type is its topic** — there are
no string topics.

Instances never touch the Bus. They open their own client through the
`RuntimeAccess` they get in `Provision`:

```go
func (Module) Provision(spec feconfig.InstanceSpec, rt fe.RuntimeAccess) (fe.Instance, error) {
	client, err := rt.BusClient("zone-watcher")
	if err != nil {
		return nil, err
	}
	return &Instance{client: client}, nil
}
```

Each call returns a **fresh** client owned by the caller. Closing it is
optional: the Bus closes any client still open when the Runtime stops.

## Publish and subscribe

```go
type ZoneUpdated struct{ Zone string }

// in Instance.Start:
pub, err := i.client.NewPublisher[ZoneUpdated]()
sub, err := i.client.Subscribe[ZoneUpdated](eventbus.SubscribeOptions{})
// or, for a callback:
subFn, err := i.client.SubscribeFunc[ZoneUpdated](func(e ZoneUpdated) {
	// ...
}, eventbus.SubscribeOptions{})

pub.Publish(ZoneUpdated{Zone: "example.com"})
for e := range sub.Events() { // channel form
	_ = e
}
```

A client holds **at most one publisher and one subscriber per type**; a second
of either fails with `ErrPublisherExists` / `ErrSubscriberExists`.

## Delivery

- **Asynchronous.** `Publish` enqueues and returns; a worker goroutine owned by
  the *subscribing* client delivers.
- **Ordered per client.** A client's events are delivered one at a time in
  global publish order — even across the different types it subscribes to.
- **Reentrant.** Publishing from inside a callback is safe; it is deferred, so
  it cannot deadlock the worker.
- `pub.HasSubscriber()` reports whether anyone listens, so a producer can skip
  building an event no one wants.

## Backpressure

`eventbus.SubscribeOptions{Capacity, Overflow}` controls what happens when a
subscriber falls behind. `Capacity` defaults to `16`; the queue is sized
`2×Capacity`.

| `Overflow` | On a full queue |
|---|---|
| `OverflowBlock` (default) | The publisher blocks. Backpressure is applied to the whole Bus — the router is a single goroutine — so a slow subscriber slows every publisher, not just its own. |
| `OverflowDropNewest` | The incoming event is dropped; the publisher never blocks. |
| `OverflowDropOldest` | The backlog is discarded and the newest event kept. |

The Bus's own router queue is sized by `options.bus.router_capacity` in the
MachineConfig (default `128`).

## Setting it up

Both live in the MachineConfig's `options.bus`:

```json
{ "options": { "bus": { "router_capacity": 128, "disable": false } } }
```

A disabled Bus is still non-nil: `BusClient` returns `ErrBusDisabled`, so a module
that treats it as an optional dependency degrades cleanly.

## Errors

| Error | When |
|---|---|
| `ErrBusDisabled` | the config disabled the Bus |
| `ErrBusClosed` | the Runtime has stopped |
| `ErrClientClosed` | this client was closed |
| `ErrPublisherClosed` | `Publisher.Close` was called |
| `ErrPublisherExists`, `ErrSubscriberExists` | a second publisher/subscriber for the same type on one client |