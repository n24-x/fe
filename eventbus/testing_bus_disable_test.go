package eventbus

import (
	"errors"
	"testing"
)

// TestDisabledBusStaysNonNil: Disable must yield a usable handle, not nil.
// Callers hold the *Bus for the lifetime of the framework object that owns it
// (Runtime, App) and must never have to nil-check it.
func TestDisabledBusStaysNonNil(t *testing.T) {
	if b := NewWithOptions(BusOptions{Disable: true}); b == nil {
		t.Fatal("NewWithOptions with Disable returned a nil Bus")
	}
}

// TestDisabledBusRefusesClients: a disabled Bus refuses every client with
// ErrBusDisabled instead of panicking or handing out a client that cannot work.
func TestDisabledBusRefusesClients(t *testing.T) {
	b := NewWithOptions(BusOptions{Disable: true})

	c, err := b.NewClient("c")
	if !errors.Is(err, ErrBusDisabled) {
		t.Fatalf("NewClient on a disabled Bus: err = %v, want ErrBusDisabled", err)
	}
	if c != nil {
		t.Fatalf("NewClient on a disabled Bus: client = %v, want nil", c)
	}
}

// TestDisabledBusCloseIsNoOp: closing a disabled Bus — twice, since Close is
// defined to be idempotent — must not panic. There is no router goroutine and
// no client behind it.
func TestDisabledBusCloseIsNoOp(t *testing.T) {
	b := NewWithOptions(BusOptions{Disable: true})
	b.Close()
	b.Close()
}
