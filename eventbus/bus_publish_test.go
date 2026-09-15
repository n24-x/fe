package eventbus

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Publisher registration on a Client: the Client.pubs bookkeeping, duplicate
// detection, recreation after Close, and the errors Publish/NewPublisher report
// for closed entities or duplicate registration (sentinels, not panics; see
// errors.go).

func TestPublisherRegistered(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "pub")

	if got := c.publishTypes(); len(got) != 0 {
		t.Fatalf("publishTypes before any publisher = %v, want empty", got)
	}

	pub := mustPublisher[int](t, c)
	eventType := reflect.TypeFor[int]()

	// Registered in Client.pubs (used for duplicate detection and cascading close).
	if c.pubs[eventType] != pub.core {
		t.Fatal("publish: core not registered in client.pubs")
	}
	// publishTypes reflects the registration.
	pts := c.publishTypes()
	if len(pts) != 1 || pts[0] != eventType {
		t.Fatalf("publishTypes = %v, want [int]", pts)
	}

	pub.Close()
	assertPublisherUnregistered(t, c, eventType, "close")
}

func assertPublisherUnregistered(t *testing.T, c *Client, eventType reflect.Type, how string) {
	t.Helper()
	if _, ok := c.pubs[eventType]; ok {
		t.Fatalf("%s: client.pubs still has entry for %v", how, eventType)
	}
	if got := c.publishTypes(); len(got) != 0 {
		t.Fatalf("%s: publishTypes = %v, want empty", how, got)
	}
}

// TestDuplicatePublisherReturnsError: asking the same Client for a second
// publisher of the same type returns ErrPublisherExists. Closing the first one
// makes it possible again (see TestPublisherRecreateAfterClose).
func TestDuplicatePublisherReturnsError(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "pub")

	pub := mustPublisher[int](t, c)
	defer pub.Close()

	_, err := c.NewPublisher[int]()
	if !errors.Is(err, ErrPublisherExists) {
		t.Fatalf("duplicate publisher: err = %v, want ErrPublisherExists", err)
	}
	// The error should name the client and the type, to ease locating.
	if msg := err.Error(); !strings.Contains(msg, "pub") || !strings.Contains(msg, "int") {
		t.Fatalf("duplicate publisher: err = %q, want it to mention client and type", msg)
	}
}

func TestPublisherRecreateAfterClose(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "pub")

	pub := mustPublisher[int](t, c)
	pub.Close() // unregisters

	// After unregistering, the same type can be obtained again (the stale core
	// must not remove the new registration).
	pub2 := mustPublisher[int](t, c)
	defer pub2.Close()
	if c.pubs[reflect.TypeFor[int]()] != pub2.core {
		t.Fatal("recreated publisher not registered")
	}
	// Closing the stale core again must not remove the new registration.
	pub.Close()
	if c.pubs[reflect.TypeFor[int]()] != pub2.core {
		t.Fatal("closing stale core removed the recreated publisher")
	}
}

func TestPublisherCloseIdempotent(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "pub")

	pub := mustPublisher[int](t, c)
	pub.Close()
	pub.Close() // idempotent: must not panic
}

// TestNewPublisherOnClosedClientReturnsError: obtaining a publisher from a closed
// Client returns ErrClientClosed.
func TestNewPublisherOnClosedClientReturnsError(t *testing.T) {
	b := New()
	c := mustClient(t, b, "pub")
	c.Close()

	if _, err := c.NewPublisher[int](); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("publisher on closed client: err = %v, want ErrClientClosed", err)
	}
}

// TestPublishOnClosedPublisherReturnsError: after the publisher itself is closed
// (unregistered), publishing through it deterministically returns
// ErrPublisherClosed.
func TestPublishOnClosedPublisherReturnsError(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "pub")

	pub := mustPublisher[int](t, c)
	pub.Close() // unregisters + sets closed

	if err := pub.Publish(1); !errors.Is(err, ErrPublisherClosed) {
		t.Fatalf("publish on closed publisher: err = %v, want ErrPublisherClosed", err)
	}
}

// TestPublishOnClosedBusReturnsErrorDeterministic: after Bus.Close, Publish must
// *deterministically* return ErrBusClosed. It must never fall into the
// non-deterministic case where the buffer still has room, select picks that arm
// at random, and the event is silently swallowed without an error (the router has
// exited, so a write is a loss anyway).
func TestPublishOnClosedBusReturnsErrorDeterministic(t *testing.T) {
	b := New()
	c := mustClient(t, b, "pub")
	pub := mustPublisher[int](t, c)
	b.Close() // the client was not closed separately; closing the bus cascades

	for i := 0; i < 200; i++ {
		if err := pub.Publish(i); !errors.Is(err, ErrBusClosed) {
			t.Fatalf("publish #%d on closed bus: err = %v, want ErrBusClosed", i, err)
		}
	}
}

// TestPublishReturnsNilWhenHealthy: on the healthy path (nobody subscribed, or
// blocked on backpressure) Publish returns nil. "Nobody subscribed" is
// deliberately *not* an error (see Publisher.HasSubscriber).
//
// The three cases deliberately use *different* types. Sharing int would let the
// event from case 1 linger in router.eventInput (the subscriber set is decided
// when the router processes the event), and the subscription created in case 2
// would then receive it and cross-talk.
func TestPublishReturnsNilWhenHealthy(t *testing.T) {
	type noSubEv struct{}
	type deliveredEv struct{ N int }
	type blockedEv struct{ N int }

	b := New()
	defer b.Close()

	// 1. Nobody subscribed: the event is silently dropped, but that is not an error.
	pubNoSub := mustPublisher[noSubEv](t, mustClient(t, b, "pub"))
	if err := pubNoSub.Publish(noSubEv{}); err != nil {
		t.Fatalf("publish with no subscriber: err = %v, want nil", err)
	}

	// 2. Someone is subscribed: normal delivery returns nil, and the event really
	// arrives.
	c := mustClient(t, b, "sub")
	sub := mustSubscribe[deliveredEv](t, c, SubscribeOptions{})
	defer sub.Close()
	pub := mustPublisher[deliveredEv](t, mustClient(t, b, "pub2"))
	if err := pub.Publish(deliveredEv{N: 42}); err != nil {
		t.Fatalf("publish with subscriber: err = %v, want nil", err)
	}
	if got := recv(t, sub.Events()); got.N != 42 {
		t.Fatalf("delivered = %v, want {42}", got)
	}

	// 3. Backpressure path (queue full → blocked enqueue): still returns nil.
	subBlocked := mustSubscribe[blockedEv](t, mustClient(t, b, "sub-blocked"), SubscribeOptions{Capacity: 1})
	defer subBlocked.Close()
	pubBlocked := mustPublisher[blockedEv](t, mustClient(t, b, "pub3"))
	for i := 0; i < 50; i++ {
		if err := pubBlocked.Publish(blockedEv{N: i}); err != nil {
			t.Fatalf("blocked publish #%d: err = %v, want nil", i, err)
		}
	}
}

// TestPublishReportsOutermostCloseCause: after shutdown all three close
// conditions hold at once (Bus.Close cascades Client.Close, which cascades each
// Publisher.Close), and publish must report the *outermost* cause. Otherwise
// ErrClientClosed / ErrBusClosed would be unreachable on the shutdown path and
// errors.Is would lose its ability to distinguish "my handle died (a bug)" from
// "the system is shutting down (normal)".
func TestPublishReportsOutermostCloseCause(t *testing.T) {
	t.Run("bus closed wins over cascaded publisher close", func(t *testing.T) {
		b := New()
		c := mustClient(t, b, "pub")
		pub := mustPublisher[int](t, c)
		b.Close() // cascade: client.Close → publisher.Close (pc.closed is true too)

		if err := pub.Publish(1); !errors.Is(err, ErrBusClosed) {
			t.Fatalf("err = %v, want ErrBusClosed (outermost cause)", err)
		}
	})

	t.Run("client closed wins over cascaded publisher close", func(t *testing.T) {
		b := New()
		defer b.Close()
		c := mustClient(t, b, "pub")
		pub := mustPublisher[int](t, c)
		c.Close() // cascade: publisher.Close (pc.closed is true too)

		if err := pub.Publish(1); !errors.Is(err, ErrClientClosed) {
			t.Fatalf("err = %v, want ErrClientClosed (outermost cause)", err)
		}
	})

	t.Run("publisher closed wins when rest of system is healthy", func(t *testing.T) {
		b := New()
		defer b.Close()
		c := mustClient(t, b, "pub")
		pub := mustPublisher[int](t, c)
		pub.Close() // only the publisher itself is closed; bus/client are healthy

		if err := pub.Publish(1); !errors.Is(err, ErrPublisherClosed) {
			t.Fatalf("err = %v, want ErrPublisherClosed", err)
		}
	})
}

func TestHasSubscriber(t *testing.T) {
	b := New()
	pubC := mustClient(t, b, "pub")
	subC := mustClient(t, b, "sub")

	pub := mustPublisher[int](t, pubC)
	if pub.HasSubscriber() {
		t.Fatal("HasSubscriber: true before any subscriber")
	}

	sub := mustSubscribe[int](t, subC, SubscribeOptions{})
	if !pub.HasSubscriber() {
		t.Fatal("HasSubscriber: false while subscriber exists")
	}

	sub.Close()
	if pub.HasSubscriber() {
		t.Fatal("HasSubscriber: true after last subscriber closed")
	}
	pub.Close()
}
