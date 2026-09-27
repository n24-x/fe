package runtimex

import "runtime"

// GoroutineID returns the current goroutine ID.
//
// The Go stdlib does not expose this API, so parse the first line of runtime.Stack
// output ("goroutine 123 [running]: ...").
//
// It takes a stack snapshot and costs a few hundred ns; do not use it on hot paths.
func GoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id uint64
	i := len("goroutine ")
	for ; i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}
