// Package assert provides assertions for tests. It was inspired by github.com/stretchr/testify/assert.
package assert

// TestingT is the entire interface these assertions need: a way to report a
// failure. Keeping it one method wide means anything can be asserted against —
// *testing.T, *testing.B, a mock, or a recorder in tests of these assertions.
type TestingT interface {
	Errorf(format string, args ...any)
}

// tHelper is implemented by *testing.T and friends. It is asserted for
// optionally rather than added to TestingT: widening the interface would force
// every custom TestingT to implement it, and all it buys is a better line
// number in the failure output.
type tHelper interface {
	Helper()
}
