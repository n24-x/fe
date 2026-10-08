package eventbus

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Subscription registration: the Client.subs / Bus.topics bookkeeping, option
// handling, duplicate detection, and the error returned on a closed Client.

func TestSubscribeRegistration(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")

	sub := mustSubscribe[int](t, c, SubscribeOptions{})

	eventType := reflect.TypeFor[int]()
	// Registered in two places: Client.subs (duplicate detection / cascade) and
	// Bus.topics (delivery lookup).
	core, ok := c.subs[eventType]
	if !ok {
		t.Fatal("subscribe: client.subs has no entry for int")
	}
	if got := len(b.topics[eventType]); got != 1 {
		t.Fatalf("subscribe: bus.topics[int] len = %d, want 1", got)
	}
	if core.delivery != b.topics[eventType][0] {
		t.Fatal("subscribe: core.delivery not registered in bus.topics")
	}

	// Defaults: Block policy and the default queue capacity.
	sd := core.delivery
	if sd.policy != OverflowBlock {
		t.Fatalf("subscribe: default policy = %v, want OverflowBlock", sd.policy)
	}
	if cap(sd.inbox) != 2*DefaultSubscriberCapacity {
		t.Fatalf("subscribe: default queue capacity = %d, want %d (Block backlog = 2×Capacity)",
			cap(sd.inbox), 2*DefaultSubscriberCapacity)
	}
	if sd.deliverFunc == nil {
		t.Fatal("subscribe: channel subscriber deliverFunc closure not installed")
	}

	// The facade is usable.
	if sub.Events() == nil {
		t.Fatal("subscribe: Events() channel is nil")
	}
	select {
	case <-sub.Done():
		t.Fatal("subscribe: Done() closed before Close")
	default:
	}

	sub.Close()
	assertUnsubscribed(t, b, c, eventType, "close")
}

func assertUnsubscribed(t *testing.T, b *Bus, c *Client, eventType reflect.Type, how string) {
	t.Helper()
	if _, ok := c.subs[eventType]; ok {
		t.Fatalf("%s: client.subs still has entry for %v", how, eventType)
	}
	if got := len(b.topics[eventType]); got != 0 {
		t.Fatalf("%s: bus.topics[%v] len = %d, want 0", how, eventType, got)
	}
}

func TestSubscribeOptionsApplied(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")

	// DropOldest: the queue is 2×Capacity too (the producing side blocks).
	sub1 := mustSubscribe[string](t, c, SubscribeOptions{Capacity: 7, Overflow: OverflowDropOldest})
	sd1 := c.subs[reflect.TypeFor[string]()].delivery
	if cap(sd1.inbox) != 14 {
		t.Fatalf("opts: capacity = %d, want 14 (= 2×7)", cap(sd1.inbox))
	}
	if sd1.policy != OverflowDropOldest {
		t.Fatalf("opts: policy=%v, want DropOldest", sd1.policy)
	}
	sub1.Close()

	// DropNewest
	sub2 := mustSubscribe[string](t, c, SubscribeOptions{Capacity: 2, Overflow: OverflowDropNewest})
	sd2 := c.subs[reflect.TypeFor[string]()].delivery
	if sd2.policy != OverflowDropNewest {
		t.Fatalf("opts: policy=%v, want DropNewest", sd2.policy)
	}
	sub2.Close()
}

func TestSubscribeFuncRegistration(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")
	called := false

	sf := mustSubscribeFunc[int](t, c, func(v int) { called = true }, SubscribeOptions{})
	_ = sf

	sd := c.subs[reflect.TypeFor[int]()].delivery
	if sd.deliverFunc == nil {
		t.Fatal("SubscribeFunc: deliverFunc closure not installed")
	}
	if called {
		t.Fatal("SubscribeFunc: callback invoked before any event")
	}
	sf.Close()
}

// TestDuplicateSubscribeReturnsError: subscribing twice to the same type on the
// same Client returns ErrSubscriberExists. Closing the first subscription makes
// subscribing again possible.
func TestDuplicateSubscribeReturnsError(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "a")
	sub := mustSubscribe[int](t, c, SubscribeOptions{})
	defer sub.Close()

	dup, err := c.Subscribe[int](SubscribeOptions{})
	if !errors.Is(err, ErrSubscriberExists) {
		t.Fatalf("duplicate subscribe: err = %v, want ErrSubscriberExists", err)
	}
	if dup != nil {
		t.Fatal("duplicate subscribe: returned non-nil subscriber on error")
	}
	// The error should name the client and the type, to ease locating.
	if msg := err.Error(); !strings.Contains(msg, "a") || !strings.Contains(msg, "int") {
		t.Fatalf("duplicate subscribe: err = %q, want it to mention client and type", msg)
	}

	// After unregistering, subscribing to the same type succeeds again.
	sub.Close()
	sub2 := mustSubscribe[int](t, c, SubscribeOptions{})
	sub2.Close()
}

// TestSubscribeOnClosedClientReturnsError: subscribing on a closed Client returns
// ErrClientClosed.
func TestSubscribeOnClosedClientReturnsError(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")
	c.Close()

	sub, err := c.Subscribe[int](SubscribeOptions{})
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("subscribe on closed client: err = %v, want ErrClientClosed", err)
	}
	if sub != nil {
		t.Fatal("subscribe on closed client: returned non-nil subscriber on error")
	}

	sf, ferr := c.SubscribeFunc(func(int) {}, SubscribeOptions{})
	if !errors.Is(ferr, ErrClientClosed) {
		t.Fatalf("SubscribeFunc on closed client: err = %v, want ErrClientClosed", ferr)
	}
	if sf != nil {
		t.Fatal("SubscribeFunc on closed client: returned non-nil subscriber on error")
	}
}

// TestSubscriberFuncDone: Done() on a callback subscription shares the same
// core.done as Subscriber[T], so "when is it closed" has exactly the same
// semantics — an explicit Close, Client.Close, and the Bus.Close cascade all wake
// the waiter.
func TestSubscriberFuncDone(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")
	sf := mustSubscribeFunc[int](t, c, func(int) {}, SubscribeOptions{})

	// Must not be closed before Close.
	select {
	case <-sf.Done():
		t.Fatal("subscriberfunc: Done() closed before Close")
	default:
	}

	// Closed by its own Close.
	sf.Close()
	select {
	case <-sf.Done():
	default:
		t.Fatal("subscriberfunc: Done() not closed after own Close")
	}

	// Bus.Close cascade: Bus → Client → Subscriber shuts down level by level and
	// wakes callback subscriptions as well.
	c2 := mustClient(t, b, "b")
	sf2 := mustSubscribeFunc[string](t, c2, func(string) {}, SubscribeOptions{})
	b.Close()
	select {
	case <-sf2.Done():
	default:
		t.Fatal("subscriberfunc: Done() not closed after bus close cascade")
	}
}
