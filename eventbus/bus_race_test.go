package eventbus

import (
	"sync"
	"testing"
)

// TestRaceSubscribeVsPublish: concurrent subscribe and publish must not race,
// and no event may be picked up before the registration is complete
// (deliverFunc already in place).
func TestRaceSubscribeVsPublish(t *testing.T) {
	for i := 0; i < 300; i++ {
		b := New()
		c := mustClient(t, b, "c")
		p := mustPublisher[int](t, mustClient(t, b, "p"))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = c.Subscribe[int](SubscribeOptions{}) // race case: failure is fine
		}()
		go func() {
			defer wg.Done()
			p.Publish(1)
		}()
		wg.Wait()
		b.Close()
	}
}

// TestRaceWorkerWake: the router wakes the worker as soon as the first
// subscription is registered, which must not race with the subscriber
// initializing workerWake.
func TestRaceWorkerWake(t *testing.T) {
	for i := 0; i < 300; i++ {
		b := New()
		c := mustClient(t, b, "c")
		p := mustPublisher[int](t, mustClient(t, b, "p"))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				p.Publish(j)
			}
		}()
		go func() {
			defer wg.Done()
			_, _ = c.Subscribe[int](SubscribeOptions{}) // race case: failure is fine
		}()
		wg.Wait()
		b.Close()
	}
}
