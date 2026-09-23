package assert

import (
	"bytes"
	"reflect"
	"strings"
)

// objectsAreEqual reports whether two values are equal.
//
// It is reflect.DeepEqual with two adjustments:
//
//   - A plain nil on either side is compared directly, without reflection.
//     DeepEqual(nil, nil) is true, but a nil interface and a nil *T holding
//     nothing are different values, and routing them through reflection hides
//     which one showed up.
//
//   - Two []byte compare by content, and nil is kept distinct from empty. The
//     nil check is not redundant with bytes.Equal, which calls nil and an empty
//     slice equal; this keeps the answer consistent with DeepEqual, which is
//     the documented baseline.
func objectsAreEqual(expected, actual any) bool {
	if expected == nil || actual == nil {
		return expected == actual
	}

	exp, ok := expected.([]byte)
	if !ok {
		return reflect.DeepEqual(expected, actual)
	}
	act, ok := actual.([]byte)
	if !ok {
		return false
	}
	if exp == nil || act == nil {
		return exp == nil && act == nil
	}
	return bytes.Equal(exp, act)
}

// getLen reports the length of a value that supports len().
//
// The recover is load-bearing rather than defensive: reflect's Len panics for
// kinds that have no length (int, bool, struct, ...), and that case has to come
// back as a reported failure instead of a crash. Deferring it also covers a nil
// argument, whose reflect.Value is invalid.
func getLen(x any) (length int, ok bool) {
	v := reflect.ValueOf(x)
	defer func() {
		ok = recover() == nil
	}()
	return v.Len(), true
}

// containsElement reports (applicable, found) for "list contains element":
// a substring for strings, a key for maps, an element for slices and arrays.
//
// As in getLen, the recover turns "this type has no containment" into a
// reported failure rather than a panic from reflect.
func containsElement(list, element any) (ok, found bool) {
	listType := reflect.TypeOf(list)
	if listType == nil {
		return false, false
	}

	defer func() {
		if recover() != nil {
			ok, found = false, false
		}
	}()

	listValue := reflect.ValueOf(list)
	switch listType.Kind() {
	case reflect.String:
		// Comparing against reflect rather than asserting to string keeps
		// named string types (type Name string) working, and makes a non-string
		// element "not applicable" instead of silently "not found".
		el := reflect.ValueOf(element)
		if el.Kind() != reflect.String {
			return false, false
		}
		return true, strings.Contains(listValue.String(), el.String())

	case reflect.Map:
		for _, key := range listValue.MapKeys() {
			if objectsAreEqual(key.Interface(), element) {
				return true, true
			}
		}
		return true, false

	default:
		for i := range listValue.Len() {
			if objectsAreEqual(listValue.Index(i).Interface(), element) {
				return true, true
			}
		}
		return true, false
	}
}

// isFunc reports whether v is a non-nil function, for which DeepEqual is not a
// meaningful equality test.
func isFunc(v any) bool {
	return v != nil && reflect.TypeOf(v).Kind() == reflect.Func
}
