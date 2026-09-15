package eventbus

import (
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

// Overflow policies and backpressure.
//
// Every subscription queue (except DropNewest, see TestQueueCapacityByPolicy) is
// sized 2×Capacity. 2×Capacity is the *backpressure threshold*, not a read
// buffer plus a write buffer: reads and writes share the same channel, so the
// publisher is only blocked once the total backlog reaches that size.

// TestQueueFIFOAndCapacity: white-box — queue capacity is 2×Capacity, and
// ordering is strict FIFO even at the boundary between the fast path and the
// backpressure path.
func TestQueueFIFOAndCapacity(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")
	eventType := reflect.TypeFor[int]()

	// Capacity 1 → queue capacity 2.
	sd := newSubscriberDelivery(c, SubscribeOptions{Capacity: 1, Overflow: OverflowBlock})
	if got := cap(sd.inbox); got != 2 {
		t.Fatalf("cap(sd.inbox) = %d, want 2", got)
	}
	r := &router{bus: b}

	// The first two events take the non-blocking fast path.
	if !r.submit(sd, publishedEvent{seq: 1, eventType: eventType, event: 1}) {
		t.Fatal("submit ev1: should succeed")
	}
	if !r.submit(sd, publishedEvent{seq: 2, eventType: eventType, event: 2}) {
		t.Fatal("submit ev2: should succeed")
	}
	if len(sd.inbox) != 2 {
		t.Fatalf("queue depth = %d, want 2", len(sd.inbox))
	}

	// Third event: the queue is full, so this takes the blocking path (run it on
	// its own goroutine to avoid stalling the test).
	blocked := make(chan struct{})
	go func() {
		r.submit(sd, publishedEvent{seq: 3, eventType: eventType, event: 3})
		close(blocked)
	}()

	// Consuming ev1 frees a slot, letting the blocked ev3 enqueue at the tail.
	if got := <-sd.inbox; got.seq != 1 {
		t.Fatalf("first consumed seq = %d, want 1 (FIFO)", got.seq)
	}
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked submit did not resume after a slot was freed")
	}

	// FIFO across the boundary: ev2 (fast path) must come before ev3 (blocked path).
	if got := <-sd.inbox; got.seq != 2 {
		t.Fatalf("second consumed seq = %d, want 2 (FIFO at the boundary)", got.seq)
	}
	if got := <-sd.inbox; got.seq != 3 {
		t.Fatalf("third consumed seq = %d, want 3", got.seq)
	}
	if len(sd.inbox) != 0 {
		t.Fatalf("queue should be empty, got %d", len(sd.inbox))
	}
}

// TestQueueBlocksOnlyWhenFull: the subscription queue must really buffer up to
// 2×Capacity (not just one event), and must block only once it is genuinely full.
//
// White-box: with nobody consuming, 2×capacity consecutive submits must all
// succeed; one more must block; once a slot is freed the channel itself wakes the
// blocked submit (no external signal needed), all the while preserving FIFO.
func TestQueueBlocksOnlyWhenFull(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")
	eventType := reflect.TypeFor[int]()

	const capacity = 4
	sd := newSubscriberDelivery(c, SubscribeOptions{Capacity: capacity, Overflow: OverflowBlock})
	if got := cap(sd.inbox); got != 2*capacity {
		t.Fatalf("queue capacity = %d, want %d (= 2×Capacity)", got, 2*capacity)
	}
	r := &router{bus: b}

	// With nobody consuming, submit 2×capacity events (values 0..7): all must
	// succeed without blocking.
	for i := 0; i < 2*capacity; i++ {
		done := make(chan struct{})
		go func(v int) {
			r.submit(sd, publishedEvent{eventType: eventType, event: v})
			close(done)
		}(i)
		select {
		case <-done:
		case <-time.After(300 * time.Millisecond):
			t.Fatalf("submit #%d blocked (actual depth %d, want up to %d)",
				i+1, len(sd.inbox), 2*capacity)
		}
	}
	if len(sd.inbox) != 2*capacity {
		t.Fatalf("queue depth = %d, want %d", len(sd.inbox), 2*capacity)
	}

	// The queue is now full: a further submit must block (this is the real
	// backpressure point).
	blocked := make(chan struct{})
	go func() {
		r.submit(sd, publishedEvent{eventType: eventType, event: 999})
		close(blocked)
	}()
	select {
	case <-blocked:
		t.Fatal("submit on a full queue should block (backpressure point), but did not")
	case <-time.After(200 * time.Millisecond):
	}

	// Consume one event to free a slot: the blocked submit must be woken by the
	// channel itself and complete.
	if got := <-sd.inbox; got.event != 0 {
		t.Fatalf("first consumed event = %d, want 0", got.event)
	}
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked submit did not resume after a slot was freed")
	}

	// FIFO check: after consuming 0 the queue should be [1..7] followed by 999
	// (999 was woken mid-block and only then enqueued, so it must go last).
	wantCh := []int{1, 2, 3, 4, 5, 6, 7, 999}
	if len(sd.inbox) != len(wantCh) {
		t.Fatalf("queue length = %d, want %d", len(sd.inbox), len(wantCh))
	}
	for i, want := range wantCh {
		if got := (<-sd.inbox).event.(int); got != want {
			t.Fatalf("queue[%d] = %d, want %d (must be FIFO; 999 must not jump the queue)", i, got, want)
		}
	}
}

// TestQueueCapacityByPolicy: queue capacity differs by overflow policy — only
// DropNewest stays at Capacity (its contract *is* "drop this event when full",
// so enlarging the queue would silently move the drop threshold).
func TestQueueCapacityByPolicy(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")

	for _, tc := range []struct {
		policy OverflowPolicy
		want   int
	}{
		{OverflowBlock, 6},      // 2×3
		{OverflowDropOldest, 6}, // 2×3
		{OverflowDropNewest, 3}, // not enlarged
	} {
		sd := newSubscriberDelivery(c, SubscribeOptions{Capacity: 3, Overflow: tc.policy})
		if got := cap(sd.inbox); got != tc.want {
			t.Fatalf("policy %v: cap(sd.inbox) = %d, want %d", tc.policy, got, tc.want)
		}
	}
}

// TestQueueLagToleranceEndToEnd: with a stalled subscriber the publisher should
// be able to accept noticeably more events before blocking than a single queue
// would allow.
//
// Total buffer = eventInput (RouterCapacity) + inbox (2×Capacity) + 1 event held
// by the worker. With routerCap=32 / subCap=8 that is ~49; the criterion below is
// 45, which leaves margin on both sides.
func TestQueueLagToleranceEndToEnd(t *testing.T) {
	const routerCap, subCap = 32, 8
	const publishes = 45 // < ~49 (all buffered); a much larger value would block

	b := NewWithOptions(BusOptions{RouterCapacity: routerCap})
	defer b.Close()

	// Capacity 8 + Block, and nobody consuming → the worker takes one event and
	// then blocks while delivering it.
	mustSubscribe[int](t, mustClient(t, b, "sub"), SubscribeOptions{
		Capacity: subCap, Overflow: OverflowBlock})
	pub := mustPublisher[int](t, mustClient(t, b, "pub"))

	// All publishes must finish within the timeout (the count is bounded, so no
	// goroutine is left blocked and Close will not trip over a late publish).
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for i := 0; i < publishes; i++ {
			pub.Publish(i)
		}
	}()

	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatalf("publisher blocked after %d events: the 2×Capacity buffer is not in effect", publishes)
	}
}

// TestDropNewestOverflow: on a full queue the newest event is dropped and the
// publisher never blocks.
func TestDropNewestOverflow(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	// Capacity 2 + DropNewest; nobody reads after subscribing, so the worker
	// blocks on the first delivery and the queue backs up.
	sub := mustSubscribe[int](t, subC, SubscribeOptions{Capacity: 2, Overflow: OverflowDropNewest})
	defer sub.Close()
	pub := mustPublisher[int](t, pubC)

	pub.Publish(1)                    // taken by the worker, which then blocks delivering it
	time.Sleep(50 * time.Millisecond) // make sure the worker is blocked on 1
	for i := 2; i <= 10; i++ {
		pub.Publish(i)
	}

	// Routing is asynchronous, so poll until the drop counter settles: the worker
	// holds 1 (blocked delivering), the inbox (capacity 2) holds 2 and 3, and
	// 4..10 (7 events) are dropped.
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) && b.Stats().DroppedTotal != 7 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := b.Stats().DroppedTotal; got != 7 {
		t.Fatalf("DroppedTotal = %d, want 7", got)
	}

	// Reading: first the 1 held by the worker, then 2 from the queue.
	if got := recv(t, sub.Events()); got != 1 {
		t.Fatalf("first received = %d, want 1", got)
	}
	if got := recv(t, sub.Events()); got != 2 {
		t.Fatalf("second received = %d, want 2", got)
	}
	if got := recv(t, sub.Events()); got != 3 {
		t.Fatalf("third received = %d, want 3", got)
	}
}

// TestDropNewestSubmitUnit: white-box — the router drops the newest event and
// counts it when the queue is full.
func TestDropNewestSubmitUnit(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")

	et := reflect.TypeFor[int]()
	sd := newSubscriberDelivery(c, SubscribeOptions{Capacity: 2, Overflow: OverflowDropNewest})
	sd.inbox <- publishedEvent{seq: 1, eventType: et, event: 1}
	sd.inbox <- publishedEvent{seq: 2, eventType: et, event: 2}

	r := &router{bus: b}
	if r.submit(sd, publishedEvent{seq: 3, eventType: et, event: 3}) {
		t.Fatal("submit on full DropNewest queue should return false")
	}
	if sd.dropped.Load() != 1 {
		t.Fatalf("sd.dropped = %d, want 1", sd.dropped.Load())
	}
}

// TestDropOldestCoalesceUnit: white-box — the consuming side drains the queue in
// one go, dropping the old and keeping the newest.
func TestDropOldestCoalesceUnit(t *testing.T) {
	b := New()
	defer b.Close()
	c := mustClient(t, b, "c")

	et := reflect.TypeFor[int]()
	sd := newSubscriberDelivery(c, SubscribeOptions{Capacity: 3, Overflow: OverflowDropOldest})
	for i := 1; i <= 3; i++ {
		sd.inbox <- publishedEvent{seq: uint64(i), eventType: et, event: i}
	}

	p := &clientWorker{client: c}
	ev, ok := p.pull(sd)
	if !ok {
		t.Fatal("pull on non-empty DropOldest queue should succeed")
	}
	if ev.seq != 3 {
		t.Fatalf("DropOldest kept seq %d, want 3 (newest)", ev.seq)
	}
	if sd.dropped.Load() != 2 {
		t.Fatalf("sd.dropped = %d, want 2", sd.dropped.Load())
	}
}

// TestDropOldestPublic: a DropOldest subscription eventually receives the latest
// state, with non-decreasing values along the way.
func TestDropOldestPublic(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	var mu sync.Mutex
	var got []int
	var once sync.Once
	done := make(chan struct{})

	sf := mustSubscribeFunc[int](t, subC, func(v int) {
		mu.Lock()
		got = append(got, v)
		if v == 20 {
			once.Do(func() { close(done) })
		}
		mu.Unlock()
	}, SubscribeOptions{Capacity: 2, Overflow: OverflowDropOldest})
	defer sf.Close()

	pub := mustPublisher[int](t, pubC)
	for i := 1; i <= 20; i++ {
		pub.Publish(i)
	}

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatalf("timeout: never received final state 20 (got %v)", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if got[len(got)-1] != 20 {
		t.Fatalf("last received = %d, want 20", got[len(got)-1])
	}
	if !slices.IsSorted(got) {
		t.Fatalf("received out of order: %v", got)
	}
}

// TestBlockBackpressureStillApplies: Block semantics hold — a slow consumer (one
// that never reads) blocks the publisher once the total backlog exceeds the
// budget, without dropping events; after the consumer resumes every event
// arrives in order.
func TestBlockBackpressureStillApplies(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	sub := mustSubscribe[int](t, subC, SubscribeOptions{Capacity: 2, Overflow: OverflowBlock})
	defer sub.Close()
	pub := mustPublisher[int](t, pubC)

	// Far above the router capacity (DefaultRouterCapacity = 128), so publisher
	// side backpressure is guaranteed to kick in.
	const n = 4000
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 1; i <= n; i++ {
			pub.Publish(i)
		}
	}()

	// The consumer has not read anything yet: the publisher should be blocked by
	// backpressure (rather than dropping events or deadlocking).
	select {
	case <-done:
		t.Fatal("publisher finished while subscriber never consumed (no backpressure)")
	case <-time.After(200 * time.Millisecond):
	}

	// Resume consuming: every event arrives in order.
	for i := 1; i <= n; i++ {
		if got := recv(t, sub.Events()); got != i {
			t.Fatalf("received %d, want %d", got, i)
		}
	}
	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("publisher stuck after consumer resumed")
	}
}
