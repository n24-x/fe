package eventbus

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCloseFromCallbackNoDeadlock: closing the subscription's own Client, or the
// whole Bus, from inside a subscription callback (i.e. on the worker goroutine)
// must return; it must not deadlock waiting on itself.
func TestCloseFromCallbackNoDeadlock(t *testing.T) {
	t.Run("client", func(t *testing.T) {
		b := New()
		defer b.Close()
		c := mustClient(t, b, "c")

		done := make(chan struct{})
		sf := mustSubscribeFunc[int](t, c, func(int) {
			c.Close() // close our own Client from inside the callback
			close(done)
		}, SubscribeOptions{})
		defer sf.Close()

		mustPublisher[int](t, mustClient(t, b, "p")).Publish(1)
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatal("Client.Close inside its own callback deadlocked")
		}
	})

	t.Run("bus", func(t *testing.T) {
		b := New()
		c := mustClient(t, b, "c")

		done := make(chan struct{})
		sf := mustSubscribeFunc[int](t, c, func(int) {
			b.Close() // close the bus from inside the callback
			close(done)
		}, SubscribeOptions{})
		defer sf.Close()

		mustPublisher[int](t, c).Publish(1)
		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Fatal("Bus.Close inside a callback deadlocked")
		}
	})
}

// TestNoDeliveryAfterClose: once Close has returned, a callback-based
// subscription must not be delivered to any more. The worker is blocked inside
// the callback here, so events already queued while unsubscribing must not be
// delivered either.
func TestNoDeliveryAfterClose(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")

	var mu sync.Mutex
	var got []int
	entered := make(chan struct{})
	release := make(chan struct{})
	var first atomic.Bool

	sf := mustSubscribeFunc[int](t, c, func(v int) {
		if first.CompareAndSwap(false, true) {
			close(entered)
			<-release // block the worker: opens the unsubscribe-vs-delivery window
		}
		mu.Lock()
		got = append(got, v)
		mu.Unlock()
	}, SubscribeOptions{})
	defer sf.Close()

	pub := mustPublisher[int](t, mustClient(t, b, "p"))
	pub.Publish(1)
	select {
	case <-entered:
	case <-time.After(testTimeout):
		t.Fatal("callback never entered")
	}
	pub.Publish(2)                    // queued for the subscription (worker is busy)
	time.Sleep(50 * time.Millisecond) // wait for the router to finish enqueueing
	sf.Close()                        // unsubscribe
	close(release)

	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	for _, v := range got {
		if v == 2 {
			t.Fatal("event delivered to callback after Close returned")
		}
	}
}

// TestClientCloseWaitsForInFlightCallback: Client.Close is a synchronous barrier
// — when it returns, this Client's worker has stopped and no callback can still
// be running. That lets callers who close outside a callback tear down their own
// state safely.
//
// This pins `if w != nil && !c.bus.onWorker(w) { <-w.done }` in Client.Close:
// removing that wait (or onWorker) makes this test fail — Close would return
// before the callback finished.
func TestClientCloseWaitsForInFlightCallback(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")
	pub := mustPublisher[int](t, mustClient(t, b, "p"))

	var closeReturned atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})

	sf := mustSubscribeFunc[int](t, c, func(int) {
		close(entered)
		<-release // stall: keep the worker inside the callback
	}, SubscribeOptions{})
	defer sf.Close()

	if err := pub.Publish(1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(testTimeout):
		t.Fatal("callback never entered")
	}

	closed := make(chan struct{})
	go func() {
		c.Close()
		closeReturned.Store(true)
		close(closed)
	}()

	// While the callback is still stuck, Close must not return (it is waiting for
	// the worker to exit).
	time.Sleep(50 * time.Millisecond)
	returnedEarly := closeReturned.Load()

	close(release) // let the callback finish so no goroutine leaks
	select {
	case <-closed:
	case <-time.After(testTimeout):
		t.Fatal("Client.Close never returned")
	}
	if returnedEarly {
		t.Fatal("Client.Close returned while a callback was still running: " +
			"the synchronous barrier is broken (was onWorker / <-w.done removed?)")
	}
}

// TestClientCloseCascadesToSubscribers: Client.Close closes every subscription of
// that Client and clears its entries from Bus.topics.
func TestClientCloseCascadesToSubscribers(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")
	s1 := mustSubscribe[int](t, c, SubscribeOptions{})
	s2 := mustSubscribe[string](t, c, SubscribeOptions{})
	sf := mustSubscribeFunc[bool](t, c, func(bool) {}, SubscribeOptions{})
	defer s1.Close() // no-op after the client is closed

	c.Close()

	for name, d := range map[string]<-chan struct{}{
		"int":    s1.Done(),
		"string": s2.Done(),
		"bool":   sf.core.done,
	} {
		select {
		case <-d:
		default:
			t.Fatalf("client close: subscriber %s Done not closed", name)
		}
	}
	if len(b.topics) != 0 {
		t.Fatalf("client close: bus.topics not empty, got %d topics", len(b.topics))
	}

	// Idempotent.
	c.Close()
}

// TestBusCloseCascades: closing the Bus closes its clients and their
// subscriptions, and the bus itself rejects new clients afterwards.
func TestBusCloseCascades(t *testing.T) {
	b := New()
	c := mustClient(t, b, "a")
	sub := mustSubscribe[int](t, c, SubscribeOptions{})

	b.Close()

	select {
	case <-sub.Done():
	default:
		t.Fatal("bus close: subscriber Done not closed")
	}
	select {
	case <-c.Done():
	default:
		t.Fatal("bus close: client Done not closed")
	}
	// The bus is closed: creating a client returns ErrBusClosed.
	if c2, err := b.NewClient("b"); !errors.Is(err, ErrBusClosed) {
		t.Fatalf("client on closed bus: err = %v, want ErrBusClosed", err)
	} else if c2 != nil {
		t.Fatal("client on closed bus: returned non-nil client on error")
	}
	b.Close() // idempotent
}

// TestClientCloseClosesPublishers: closing a Client closes the publishers it owns,
// so publishing through them reports ErrClientClosed afterwards, and constructing
// a new publisher on the closed Client reports the same sentinel.
func TestClientCloseClosesPublishers(t *testing.T) {
	b := New()
	c := mustClient(t, b, "pub")

	pub := mustPublisher[int](t, c)
	c.Close() // cascades to the publisher

	if err := pub.Publish(1); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("publish on closed client: err = %v, want ErrClientClosed", err)
	}
	// *Constructing* a publisher on a closed Client returns ErrClientClosed too:
	// it is the same shutdown race as Publish (Client.Close may run concurrently
	// with NewPublisher), so it reuses the same sentinel and callers can detect
	// "the system is shutting down" with one check.
	if _, err := c.NewPublisher[int](); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("new publisher on closed client: err = %v, want ErrClientClosed", err)
	}
	// An explicit Close after the cascade is idempotent.
	pub.Close()
}
