// Package assert provides assertions for tests: a deliberately narrow
// subset of github.com/stretchr/testify/assert.
//
// See wiki/testify_api.md for which assertions were kept and the usage data
// behind each cut. Three differences are worth knowing up front:
//
//   - Only the trailing `msgAndArgs ...any` form of a message exists. There
//     are no `f` variants and no object-style assertions: across five real
//     projects the `f` variants were 12 of ~1800 calls (0.7%) and the object
//     style was used not once. Dropping both removes about half of testify's
//     code and its whole code-generation pipeline.
//
//   - A failed comparison prints the two values with %#v instead of a unified
//     diff. testify vendors ~2700 lines (spew + difflib) to produce that diff;
//     for a framework this size the two values are enough. %#v was chosen over
//     %+v because it names a struct's type and quotes strings, so a difference
//     in whitespace is visible. The cost is real: a long multi-line difference
//     is harder to read here than one drawn as a diff.
//
//     Values are still bounded, but at ~32 KiB rather than at a readability
//     limit (see maxPrintedValue): the testing package drops any output line
//     over 64 KiB, so without a cap a single enormous value would cost the
//     whole failure message rather than merely clutter it.
//
//   - There is no Error Trace label. Testify walks the stack itself and skips
//     frames by matching its own directory names; t.Helper() already makes the
//     testing package report the caller's line, so the walk would only print
//     the same location twice.
//
// Every assertion reports a failure through fail and returns true or false, so
// the return value can be used as a guard:
//
//	if assert.NoError(t, err) {
//		assert.Equal(t, want, got)
//	}
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
