package assert

import "fmt"

// The T1 tier of the extraction in wiki/testify_api.md: the seven assertions
// every one of the five reference projects used. The rest of the tiers are
// deliberately absent — see that document for what they are and the usage data
// behind the ordering.

// Equal asserts that expected and actual are equal. Equality is
// reflect.DeepEqual with the two adjustments described on objectsAreEqual: nil
// compared directly, and []byte compared by content.
//
//	assert.Equal(t, "hello", greeting)
func Equal(t TestingT, expected, actual any, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	// reflect.DeepEqual reports two identical non-nil functions as unequal,
	// since functions are only comparable to nil. The failure would read
	// "expected 0x4711f0, actual 0x4711f0", which is worse than useless; say
	// what is actually wrong instead.
	if isFunc(expected) || isFunc(actual) {
		return fail(t, "cannot compare func values with Equal: they are only comparable to nil",
			msgAndArgs...)
	}

	if !objectsAreEqual(expected, actual) {
		exp, act := formatValues(expected, actual)
		return fail(t, fmt.Sprintf("Not equal:\nexpected: %s\nactual  : %s", exp, act), msgAndArgs...)
	}
	return true
}

// True asserts that value is true.
//
//	assert.True(t, ok)
func True(t TestingT, value bool, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if !value {
		return fail(t, "Should be true", msgAndArgs...)
	}
	return true
}

// False asserts that value is false.
//
//	assert.False(t, done)
func False(t TestingT, value bool, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if value {
		return fail(t, "Should be false", msgAndArgs...)
	}
	return true
}

// NoError asserts that err is nil.
//
//	got, err := f()
//	if !assert.NoError(t, err) {
//		return
//	}
func NoError(t TestingT, err error, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if err != nil {
		// %+v rather than %#v: an error prints its message through Error(), and
		// %#v would show its unexported fields instead.
		return fail(t, "Received unexpected error:\n"+formatBounded("%+v", err), msgAndArgs...)
	}
	return true
}

// Error asserts that err is not nil. It does not look at what the error is:
// use ErrorIs or ErrorContains when the kind of error is the point. Those are a
// later tier.
//
//	assert.Error(t, err)
func Error(t TestingT, err error, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}
	if err == nil {
		return fail(t, "An error is expected but got nil", msgAndArgs...)
	}
	return true
}

// Contains asserts that s contains element: a substring for strings, a key for
// maps, an element for slices and arrays.
//
//	assert.Contains(t, "hello world", "world")
//	assert.Contains(t, []int{1, 2, 3}, 2)
//	assert.Contains(t, map[string]int{"a": 1}, "a")
func Contains(t TestingT, s, element any, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	ok, found := containsElement(s, element)
	if !ok {
		return fail(t, fmt.Sprintf("%T is not something that can contain %s", s, formatBounded("%#v", element)),
			msgAndArgs...)
	}
	if !found {
		return fail(t, fmt.Sprintf("%s does not contain %s", formatBounded("%#v", s), formatBounded("%#v", element)),
			msgAndArgs...)
	}
	return true
}

// Len asserts that object has the given length. It fails, rather than
// panicking, for a type that len() does not accept.
//
//	assert.Len(t, names, 3)
func Len(t TestingT, object any, length int, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	l, ok := getLen(object)
	if !ok {
		return fail(t, fmt.Sprintf("%T does not have a length", object), msgAndArgs...)
	}
	if l != length {
		// The value is deliberately not printed. Its length is the whole
		// question, and a wrong length is most likely on the largest values —
		// exactly the ones whose dump would bury the two numbers that matter.
		return fail(t, fmt.Sprintf("%T should have %d item(s), but has %d", object, length, l),
			msgAndArgs...)
	}
	return true
}
