package eventbus

import (
	"sync/atomic"
	"testing"
	"time"
)

// Benchmarks for the two hot-path optimisations: zero-allocation routing
// snapshots and a zero-allocation steady-state worker.
// Run with `go test -bench . -benchmem`.

type benchEv struct{ N int }

var benchSubNames = [8]string{"s0", "s1", "s2", "s3", "s4", "s5", "s6", "s7"}

func waitReceived(b *testing.B, got *atomic.Int64, want int64) {
	b.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for got.Load() < want {
		if time.Now().After(deadline) {
			b.Fatalf("timeout: received %d/%d", got.Load(), want)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// BenchmarkPublishDeliverOne: one publisher and one callback subscription,
// measuring end-to-end throughput across publish → router (deliverySnapshot) →
// worker merge and dispatch.
func BenchmarkPublishDeliverOne(b *testing.B) {
	bus := New()
	defer bus.Close()

	var got atomic.Int64
	sf := mustSubscribeFuncB[benchEv](b, mustClientB(b, bus, "sub"), func(benchEv) { got.Add(1) }, SubscribeOptions{})
	defer sf.Close()
	pub := mustPublisherB[benchEv](b, mustClientB(b, bus, "pub"))

	pub.Publish(benchEv{}) // warm up: start the worker
	waitReceived(b, &got, 1)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.Publish(benchEv{N: i})
	}
	waitReceived(b, &got, int64(b.N)+1)
	b.StopTimer()
}

// BenchmarkPublishDeliverEight: one publisher and eight subscription Clients,
// amplifying the routing snapshot cost — each event is enqueued once per
// subscription.
func BenchmarkPublishDeliverEight(b *testing.B) {
	bus := New()
	defer bus.Close()

	var got atomic.Int64
	for i := range benchSubNames {
		sf := mustSubscribeFuncB[benchEv](b, mustClientB(b, bus, benchSubNames[i]), func(benchEv) { got.Add(1) }, SubscribeOptions{})
		defer sf.Close()
	}
	pub := mustPublisherB[benchEv](b, mustClientB(b, bus, "pub"))

	pub.Publish(benchEv{})
	waitReceived(b, &got, 8)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pub.Publish(benchEv{N: i})
	}
	waitReceived(b, &got, int64(8*b.N)+8)
	b.StopTimer()
}
