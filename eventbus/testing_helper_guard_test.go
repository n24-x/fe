package eventbus

import (
	"errors"
	"testing"
)

// Nil receiver guards on publisherCore / subscriberCore / subscriberDelivery.
//
// None of these cores can be nil through the public API (NewPublisher and
// Subscribe always set one), so the guards are a defensive convention rather
// than a reachable path. Since they exist they should be covered — otherwise
// they are "Schrödinger's guards": neither verified nor safely deletable.
//
// Convention: a nil core means "no publisher / no subscription", and methods
// return the zero value safely instead of crashing.

func TestNilPublisherCoreGuards(t *testing.T) {
	var pc *publisherCore

	// publish: returns ErrPublisherClosed (there is no usable publisher).
	if err := pc.publish(1); !errors.Is(err, ErrPublisherClosed) {
		t.Fatalf("nil core publish: err = %v, want ErrPublisherClosed", err)
	}

	// hasSubscriber: nobody subscribed → false.
	if pc.hasSubscriber() {
		t.Fatal("nil core hasSubscriber: got true, want false")
	}

	// close: a no-op, must not crash.
	pc.close()
}

// TestPublisherCoreNilClientGuards: the core is non-nil but the client is nil
// (a half-constructed state).
func TestPublisherCoreNilClientGuards(t *testing.T) {
	pc := &publisherCore{} // client == nil

	if err := pc.publish(1); !errors.Is(err, ErrPublisherClosed) {
		t.Fatalf("nil client publish: err = %v, want ErrPublisherClosed", err)
	}
	if pc.hasSubscriber() {
		t.Fatal("nil client hasSubscriber: got true, want false")
	}
	pc.close() // must not crash (it has to check the client before calling removePublisher)

	// close must still be idempotent.
	pc.close()
	if !pc.closed.Load() {
		t.Fatal("nil client close: closed flag not set")
	}
}

func TestNilSubscriberCoreCloseGuard(t *testing.T) {
	var sc *subscriberCore
	sc.close() // a no-op, must not crash
}

// TestSubscriberCoreNilFieldGuards: the core is non-nil but its fields were never
// wired up (unregister / done are nil) — close must tolerate that and not panic.
func TestSubscriberCoreNilFieldGuards(t *testing.T) {
	sc := &subscriberCore{} // unregister == nil, done == nil
	sc.close()              // must not panic

	if !sc.closed.Load() {
		t.Fatal("close: closed flag not set")
	}
	// Idempotent.
	sc.close()
}

func TestNilSubscriberDeliveryDropGuard(t *testing.T) {
	var sd *subscriberDelivery
	sd.drop(3) // a no-op, must not crash
}

// TestSubscriberDeliveryNilClientDropGuard: with a nil client the subscription-level
// counter should still be bumped; only the aggregation up to the bus level is
// unavailable.
func TestSubscriberDeliveryNilClientDropGuard(t *testing.T) {
	sd := &subscriberDelivery{}
	sd.drop(2)

	if got := sd.dropped.Load(); got != 2 {
		t.Fatalf("nil client drop: dropped = %d, want 2", got)
	}
}
