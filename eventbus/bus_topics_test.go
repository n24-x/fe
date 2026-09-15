package eventbus

import (
	"reflect"
	"testing"
)

// Copy-on-write invariant for Bus.topics.
//
// Why this needs a test: deliverySnapshot returns the current slice as-is (zero
// allocation) and the router then ranges over it *without holding the lock* (see
// router.route). Any modification of topics under the lock must therefore
// install a brand new backing array; otherwise the array the router is iterating
// has its contents swapped underneath it — a stale snapshot plus a data race.
//
// The trap: the most natural way to optimise removeTopicSubscriber is
// `slices.Delete(old, i, i+1)`, but that shifts elements in place (the official
// doc says "returns the modified slice") and silently breaks this invariant.
// With that version every functional test still passes (ordering, idempotence,
// empty-chain removal all behave), and -race rarely catches the window either —
// only the tests in this file do.

// TestTopicsSnapshotImmutable: after removing and adding subscriptions, a
// previously handed-out snapshot must be element-by-element unchanged.
func TestTopicsSnapshotImmutable(t *testing.T) {
	b := New()
	defer b.Close()
	et := reflect.TypeFor[int]()

	// Three subscriptions, standing in for a snapshot held by the router.
	sds := make([]*subscriberDelivery, 3)
	for i := range sds {
		c := mustClient(t, b, "c")
		core := newSubscriberCore(c, et, SubscribeOptions{})
		core.delivery.deliverFunc = func(publishedEvent) {}
		sds[i] = core.delivery
		b.addTopicSubscriber(et, core.delivery)
	}

	snap := b.deliverySnapshot(et)
	if len(snap) != 3 {
		t.Fatalf("snapshot len = %d, want 3", len(snap))
	}
	before := append([]*subscriberDelivery(nil), snap...) // remember the contents

	b.removeTopicSubscriber(et, sds[1]) // remove the middle one

	if got := len(b.deliverySnapshot(et)); got != 2 {
		t.Fatalf("live chain len = %d, want 2", got)
	}
	for i := range before {
		if snap[i] != before[i] {
			t.Fatalf("snapshot corrupted: snap[%d] went from %v to %v (copy-on-write broken, see the file header)",
				i, before[i], snap[i])
		}
	}

	b.addTopicSubscriber(et, sds[0]) // appending must not affect an existing snapshot either
	for i := range before {
		if snap[i] != before[i] {
			t.Fatalf("addTopicSubscriber rewrote an existing snapshot @%d", i)
		}
	}
}

// TestRemoveTopicSubscriberPositions: ordering and length for removing the first,
// middle, and last element, including the n=2 boundary.
func TestRemoveTopicSubscriberPositions(t *testing.T) {
	et := reflect.TypeFor[int]()
	for _, tc := range []struct {
		name string
		n    int
		del  int
	}{
		{"n=2_remove_first", 2, 0}, {"n=2_remove_last", 2, 1},
		{"n=5_remove_first", 5, 0}, {"n=5_remove_middle", 5, 2}, {"n=5_remove_last", 5, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := New()
			defer b.Close()

			sds := make([]*subscriberDelivery, tc.n)
			for i := range sds {
				c := mustClient(t, b, "c")
				core := newSubscriberCore(c, et, SubscribeOptions{})
				core.delivery.deliverFunc = func(publishedEvent) {}
				sds[i] = core.delivery
				b.addTopicSubscriber(et, core.delivery)
			}
			b.removeTopicSubscriber(et, sds[tc.del])

			got := b.deliverySnapshot(et)
			var want []*subscriberDelivery
			for i, sd := range sds {
				if i != tc.del {
					want = append(want, sd)
				}
			}
			if len(got) != len(want) {
				t.Fatalf("len = %d, want %d", len(got), len(want))
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("ordering broken @%d", i)
				}
			}
		})
	}
}

// TestRemoveTopicSubscriberIdempotent: removing twice is idempotent, and removing
// the last subscriber deletes the topic entry.
func TestRemoveTopicSubscriberIdempotent(t *testing.T) {
	b := New()
	defer b.Close()
	et := reflect.TypeFor[int]()

	const n = 3
	sds := make([]*subscriberDelivery, n)
	for i := range sds {
		c := mustClient(t, b, "c")
		core := newSubscriberCore(c, et, SubscribeOptions{})
		core.delivery.deliverFunc = func(publishedEvent) {}
		sds[i] = core.delivery
		b.addTopicSubscriber(et, core.delivery)
	}

	// Removing a delivery that is not in the chain: silently a no-op.
	stranger := mustClient(t, b, "stranger")
	other := newSubscriberCore(stranger, et, SubscribeOptions{})
	b.removeTopicSubscriber(et, other.delivery)
	if got := len(b.deliverySnapshot(et)); got != n {
		t.Fatalf("len after removing an unknown delivery = %d, want %d", got, n)
	}

	// Removing the same one twice.
	b.removeTopicSubscriber(et, sds[1])
	b.removeTopicSubscriber(et, sds[1])
	if got := len(b.deliverySnapshot(et)); got != n-1 {
		t.Fatalf("len after a repeated removal = %d, want %d", got, n-1)
	}

	// Down to empty: the topic entry should be deleted (no longer counted by
	// Stats.TopicCount).
	for _, i := range []int{0, 2} {
		b.removeTopicSubscriber(et, sds[i])
	}
	if _, ok := b.topics[et]; ok {
		t.Fatal("topics still holds the eventType after the chain became empty")
	}
	if got := b.Stats().TopicCount; got != 0 {
		t.Fatalf("TopicCount = %d, want 0", got)
	}
}
