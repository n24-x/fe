package eventbus

import (
	"testing"
	"time"
)

// Reentrant publishing from inside a callback.
//
// A callback runs on the Client's worker goroutine, so publishing synchronously
// from within one means the worker is both producer and consumer of the same
// router input. These tests pin that this cannot deadlock.

// TestReentrantPublishFromCallbackNoDeadlock: synchronously republishing from a
// callback (same type, chain decreasing by one) must not deadlock and every event
// must eventually be dispatched in order.
//
// Routing does not stall on a single slow subscription: a subscription queue
// holds 2×Capacity, and when a queue is full the router blocks on *that
// subscription's* channel (woken as soon as the consumer takes an event), so
// eventInput keeps its headroom and a reentrant publish from a fast callback is
// never wedged behind a stalled router.
func TestReentrantPublishFromCallbackNoDeadlock(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	delivered := 0 // only touched by the test goroutine, no lock needed
	const start = 500
	got := make(chan int, start+8)

	pub := mustPublisher[int](t, pubC) // one publisher per Client per type
	sf := mustSubscribeFunc[int](t, subC, func(v int) {
		got <- v
		if v > 0 {
			// Synchronous reentrant publish: v-1 eventually reaches this callback too.
			pub.Publish(v - 1)
		}
	}, SubscribeOptions{})
	defer sf.Close()
	pub.Publish(start)

	deadline := time.After(testTimeout)
	for i := 0; i <= start; i++ {
		select {
		case v := <-got:
			if v != start-i {
				t.Fatalf("delivery order broken: got %d, want %d", v, start-i)
			}
			delivered++
		case <-deadline:
			t.Fatalf("deadlock or stall: received %d/%d reentrant events", delivered, start+1)
		}
	}
}

// TestReentrantPublishAfterStallNoDeadlock: the harshest variant — a tiny
// eventInput (2) and subscription queue (1) plus a huge burst and a synchronous
// reentrant publish from a callback that has stalled past the point where the
// buffers are full.
//
// Without deferred publishing this must deadlock: the callback is stuck inside a
// publish on a full eventInput, so the worker cannot consume, so the router
// cannot advance, so eventInput never drains. With it, the reentrant publish is
// parked in the worker's deferred queue when eventInput is full; the callback
// returns, the worker keeps consuming, and the burst completes.
func TestReentrantPublishAfterStallNoDeadlock(t *testing.T) {
	b := NewWithOptions(BusOptions{RouterCapacity: 2})
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	release := make(chan struct{})
	entered := make(chan struct{})
	pub := mustPublisher[int](t, pubC)
	sf := mustSubscribeFunc[int](t, subC, func(v int) {
		if v == -999 {
			close(entered) // tell the test the callback has entered its stall
			// Stall: stands in for a slow callback window (during which the
			// burst fills the buffers). Also watch for shutdown so the failure
			// path can exit cleanly.
			select {
			case <-release:
			case <-subC.Done():
				return
			}
			pub.Publish(-998)
			return
		}
	}, SubscribeOptions{})
	defer sf.Close()

	// Burst publisher (far more events than eventInput plus the subscription
	// queue can hold).
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5000; i++ {
			pub.Publish(i)
		}
	}()

	pub.Publish(-999) // trigger the stalling callback
	select {
	case <-entered:
	case <-time.After(testTimeout):
		t.Fatal("callback never entered")
	}
	// Wait for the burst to fill eventInput and the subscription queue, leaving
	// the router blocked on the slow subscription.
	time.Sleep(50 * time.Millisecond)
	close(release) // the callback wakes up and republishes synchronously (eventInput is full)

	select {
	case <-done:
		// The whole burst was eventually delivered (no deadlock).
	case <-time.After(testTimeout):
		t.Fatal("deadlock: publisher stuck (reentrant publish blocked the callback)")
	}
}
