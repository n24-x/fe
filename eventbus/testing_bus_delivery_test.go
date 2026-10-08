package eventbus

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// End-to-end delivery over a real bus: the two subscription flavours
// (channel and callback), cross-type ordering, unsubscribe, and ordering
// under load.

func TestChannelDelivery(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	sub := mustSubscribe[int](t, subC, SubscribeOptions{})
	defer sub.Close()
	pub := mustPublisher[int](t, pubC)

	pub.Publish(42)
	if got := recv(t, sub.Events()); got != 42 {
		t.Fatalf("received %d, want 42", got)
	}
}

func TestFuncDelivery(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	got := make(chan string, 1)
	sf := mustSubscribeFunc[string](t, subC, func(v string) { got <- v }, SubscribeOptions{})
	defer sf.Close()

	mustPublisher[string](t, pubC).Publish("hello")
	if v := recv(t, got); v != "hello" {
		t.Fatalf("callback got %q, want hello", v)
	}
}

// TestCrossTypeOrdering: with two types subscribed on the same Client, delivery
// still happens one by one in global publish order.
func TestCrossTypeOrdering(t *testing.T) {
	type evA struct{ n int }
	type evB struct{ n int }

	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	var mu sync.Mutex
	var got []int
	var once sync.Once
	done := make(chan struct{})
	record := func(n int) {
		mu.Lock()
		got = append(got, n)
		if len(got) == 3 {
			once.Do(func() { close(done) })
		}
		mu.Unlock()
	}

	mustSubscribeFunc[evA](t, subC, func(e evA) { record(e.n) }, SubscribeOptions{})
	mustSubscribeFunc[evB](t, subC, func(e evB) { record(e.n) }, SubscribeOptions{})

	pubA := mustPublisher[evA](t, pubC)
	pubB := mustPublisher[evB](t, pubC)
	pubB.Publish(evB{1})
	pubA.Publish(evA{2})
	pubB.Publish(evB{3})

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatalf("timeout, got %v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []int{1, 2, 3}; !slices.Equal(got, want) {
		t.Fatalf("delivery order = %v, want %v", got, want)
	}
}

// TestUnsubscribeStopsDelivery: no new events arrive after unsubscribing.
func TestUnsubscribeStopsDelivery(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	sub := mustSubscribe[int](t, subC, SubscribeOptions{})
	pub := mustPublisher[int](t, pubC)

	pub.Publish(1)
	if got := recv(t, sub.Events()); got != 1 {
		t.Fatalf("first = %d, want 1", got)
	}

	sub.Close() // unsubscribe
	pub.Publish(2)
	// Give routing a moment; a delivery would land on the closed subscription
	// and be skipped.
	time.Sleep(50 * time.Millisecond)
	if _, ok := tryRecv(sub.Events()); ok {
		t.Fatal("received event after unsubscribe")
	}
}

// TestQueuePreservesOrderUnderLoad: a real bus with a small (Capacity 1) Block
// subscription and a fast consumer; after publishing N events all of them arrive
// in order (the 2×Capacity buffer neither drops nor reorders).
func TestQueuePreservesOrderUnderLoad(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	sub := mustSubscribe[int](t, subC, SubscribeOptions{Capacity: 1, Overflow: OverflowBlock})
	defer sub.Close()
	pub := mustPublisher[int](t, pubC)

	const n = 300
	go func() {
		for i := 1; i <= n; i++ {
			pub.Publish(i)
		}
	}()

	for i := 1; i <= n; i++ {
		if got := recv(t, sub.Events()); got != i {
			t.Fatalf("received %d at position %d, want in-order 1..%d", got, i, n)
		}
	}
}
