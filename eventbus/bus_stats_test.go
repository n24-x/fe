package eventbus

import (
	"testing"
	"time"
)

// TestStatsCounters: the publish/route counters track real delivery.
func TestStatsCounters(t *testing.T) {
	b := New()
	defer b.Close()
	subC := mustClient(t, b, "sub")
	pubC := mustClient(t, b, "pub")

	sub := mustSubscribe[int](t, subC, SubscribeOptions{})
	defer sub.Close()
	pub := mustPublisher[int](t, pubC)
	pub.Publish(1)
	pub.Publish(2)
	if recv(t, sub.Events()) != 1 || recv(t, sub.Events()) != 2 {
		t.Fatal("delivery failed")
	}

	// Routing is asynchronous, so poll until the counters settle.
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		s := b.Stats()
		if s.PublishedTotal == 2 && s.RoutedTotal == 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	s := b.Stats()
	t.Fatalf("stats not converged: %+v", s)
}
