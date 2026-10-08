package eventbus

import "testing"

// Worker-level deferred publishing.
//
// A callback runs on the Client's worker goroutine, so a synchronous reentrant
// publish from inside a callback is performed by the worker itself. When the
// router input is full at that moment, writing to it would block the worker and
// wedge the whole pipeline; deferPublish/flushDeferred park such events instead
// and push them once the worker loops back. This file pins those two methods'
// semantics in isolation.

// TestDeferPublishAndFlushUnit: queue semantics of deferPublish / flushDeferred.
//
// It uses a *hand-built* router/worker rather than a real Bus so that the
// assertions are fully deterministic and independent of burst timing: on a real
// Bus the router consumes eventInput whenever it likes, so "the event really was
// pushed in" cannot be observed reliably. (End-to-end non-deadlocking is covered
// by TestReentrantPublishFromCallbackNoDeadlock / ...AfterStallNoDeadlock; what
// this adds is "nothing is lost".)
func TestDeferPublishAndFlushUnit(t *testing.T) {
	r := &router{
		eventInput: make(chan publishedEvent, 4),
		done:       make(chan struct{}),
		exited:     make(chan struct{}),
	}
	w := &clientWorker{
		client:      &Client{done: make(chan struct{})},
		router:      r,
		done:        make(chan struct{}),
		deferredCap: cap(r.eventInput),
	}

	// 1. deferPublish only parks the event; it does not write eventInput.
	if err := w.deferPublish(publishedEvent{event: 1}); err != nil {
		t.Fatalf("deferPublish: %v", err)
	}
	if n := len(w.deferred); n != 1 {
		t.Fatalf("len(deferred) = %d, want 1", n)
	}
	select {
	case <-r.eventInput:
		t.Fatal("deferPublish must not write eventInput directly")
	default:
	}

	// 2. flushDeferred pushes the parked events into eventInput and clears deferred.
	w.flushDeferred()
	if n := len(w.deferred); n != 0 {
		t.Fatalf("len(deferred) = %d after flush, want 0", n)
	}
	select {
	case got := <-r.eventInput:
		if got.event != 1 {
			t.Fatalf("flushed event = %v, want 1", got.event)
		}
	default:
		t.Fatal("flushDeferred did not push the event into router.eventInput")
	}

	// 3. Multiple parked events flush in enqueue order (reentrant publishing must
	// preserve order).
	for i := 2; i <= 4; i++ {
		if err := w.deferPublish(publishedEvent{event: i}); err != nil {
			t.Fatalf("deferPublish(%d): %v", i, err)
		}
	}
	w.flushDeferred()
	for want := 2; want <= 4; want++ {
		select {
		case got := <-r.eventInput:
			if got.event != want {
				t.Fatalf("flushed event = %v, want %d (order broken)", got.event, want)
			}
		default:
			t.Fatalf("event %d not flushed", want)
		}
	}

	// 4. At deferredCap it falls back to writing eventInput directly (no more
	// parking). This is the documented extreme boundary: the number of events
	// synchronously published within a single callback should not exceed the
	// eventInput capacity.
	w.deferred = make([]publishedEvent, w.deferredCap)
	if err := w.deferPublish(publishedEvent{event: 99}); err != nil {
		t.Fatalf("deferPublish at cap: %v", err)
	}
	if n := len(w.deferred); n != w.deferredCap {
		t.Fatalf("len(deferred) = %d after cap fallback, want %d", n, w.deferredCap)
	}
	select {
	case got := <-r.eventInput:
		if got.event != 99 {
			t.Fatalf("fallback event = %v, want 99", got.event)
		}
	default:
		t.Fatal("deferPublish at cap should write eventInput directly")
	}
}
