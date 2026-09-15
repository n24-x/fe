package eventbus

import (
	"testing"
	"time"
)

// testTimeout is the deadline used by the "must happen eventually" assertions
// (declared in the baseline's bus_step4_test.go, kept here because it is shared).
const testTimeout = 3 * time.Second

// This file collects the helper functions shared across test files (it holds no
// test cases of its own).
//
// Background: NewClient / Subscribe / SubscribeFunc / NewPublisher all return
// error instead of panicking (see errors.go). Almost every call site in the test
// suite sits in a "must succeed" context (bus/client not closed, type not
// registered twice), and spelling out `if err != nil` at each one would only
// drown out the assertions themselves. They are therefore funneled through the
// must* helpers below.
//
// Cases that need to verify a *failure* should call the original method and
// assert with errors.Is directly.

// mustClient creates a new client and fails the test if that is not possible.
func mustClient(t *testing.T, b *Bus, name string) *Client {
	t.Helper()
	c, err := b.NewClient(name)
	if err != nil {
		t.Fatalf("NewClient(%q): %v", name, err)
	}
	return c
}

// mustClientB is the benchmark flavour of mustClient (uses *testing.B).
func mustClientB(b *testing.B, bus *Bus, name string) *Client {
	b.Helper()
	c, err := bus.NewClient(name)
	if err != nil {
		b.Fatalf("NewClient(%q): %v", name, err)
	}
	return c
}

// mustSubscribe creates a channel-based subscription and fails the test if that
// is not possible.
func mustSubscribe[T any](t *testing.T, c *Client, o SubscribeOptions) *Subscriber[T] {
	t.Helper()
	sub, err := c.Subscribe[T](o)
	if err != nil {
		t.Fatalf("Subscribe on client %q: %v", c.name, err)
	}
	return sub
}

// mustSubscribeFunc creates a callback-based subscription and fails the test if
// that is not possible.
func mustSubscribeFunc[T any](t *testing.T, c *Client, fn func(T), o SubscribeOptions) *SubscriberFunc[T] {
	t.Helper()
	sf, err := c.SubscribeFunc(fn, o)
	if err != nil {
		t.Fatalf("SubscribeFunc on client %q: %v", c.name, err)
	}
	return sf
}

// mustSubscribeFuncB is the benchmark flavour of mustSubscribeFunc (uses
// *testing.B).
func mustSubscribeFuncB[T any](b *testing.B, c *Client, fn func(T), o SubscribeOptions) *SubscriberFunc[T] {
	b.Helper()
	sf, err := c.SubscribeFunc(fn, o)
	if err != nil {
		b.Fatalf("SubscribeFunc on client %q: %v", c.name, err)
	}
	return sf
}

// mustPublisher returns the publisher for type T and fails the test if that is
// not possible.
func mustPublisher[T any](t *testing.T, c *Client) *Publisher[T] {
	t.Helper()
	p, err := c.NewPublisher[T]()
	if err != nil {
		t.Fatalf("NewPublisher on client %q: %v", c.name, err)
	}
	return p
}

// mustPublisherB is the benchmark flavour of mustPublisher (uses *testing.B).
func mustPublisherB[T any](b *testing.B, c *Client) *Publisher[T] {
	b.Helper()
	p, err := c.NewPublisher[T]()
	if err != nil {
		b.Fatalf("NewPublisher on client %q: %v", c.name, err)
	}
	return p
}

// recv receives one value from a read-only channel, failing on timeout.
func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for event")
		panic("unreachable")
	}
}

// tryRecv probes a channel for a value without blocking.
func tryRecv[T any](ch <-chan T) (T, bool) {
	select {
	case v := <-ch:
		return v, true
	default:
		var zero T
		return zero, false
	}
}
