package assert

import (
	"bufio"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// recorder is a TestingT that collects failures instead of reporting them, so
// that the assertions can be exercised without a *testing.T.
//
// It deliberately does not implement Helper() (nothing to skip) but does
// implement Name(), which fail asserts for optionally to add the Test label.
type recorder struct {
	name     string
	failures []string
}

func (r *recorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

func (r *recorder) Name() string { return r.name }

// only asserts exactly one failure and returns it; a second failure means the
// assertion reported twice, and none means it did not report at all.
func (r *recorder) only(t *testing.T) string {
	t.Helper()
	if len(r.failures) != 1 {
		t.Fatalf("failures = %d, want exactly 1: %q", len(r.failures), r.failures)
	}
	return r.failures[0]
}

// --- Equal ---

func TestEqual(t *testing.T) {
	tests := []struct {
		name     string
		expected any
		actual   any
		wantPass bool
		contains []string
	}{
		{name: "equal ints", expected: 1, actual: 1, wantPass: true},
		{name: "unequal ints", expected: 1, actual: 2, contains: []string{"Not equal:", "expected: 1", "actual  : 2"}},
		{name: "equal strings", expected: "a", actual: "a", wantPass: true},
		{name: "unequal strings", expected: "a", actual: "b", contains: []string{`expected: "a"`, `actual  : "b"`}},

		// A differing type is the classic "expected: 1, actual: 1" trap, so
		// the type is named whenever the two sides disagree.
		{name: "same number different type", expected: 1, actual: int64(1), contains: []string{"int(1)", "int64(1)"}},

		{name: "both nil", expected: nil, actual: nil, wantPass: true},
		{name: "nil expected", expected: nil, actual: 1, contains: []string{"expected: nil", "actual  : int(1)"}},
		{name: "nil actual", expected: 1, actual: nil, contains: []string{"expected: int(1)", "actual  : nil"}},

		{name: "equal slices", expected: []int{1, 2}, actual: []int{1, 2}, wantPass: true},
		{name: "unequal slices", expected: []int{1, 2}, actual: []int{1, 3}, contains: []string{"Not equal:"}},
		{name: "equal structs", expected: struct{ A int }{1}, actual: struct{ A int }{1}, wantPass: true},
		// %#v names the type, so the message says which struct shape was
		// compared and not just which fields differed.
		{name: "unequal structs", expected: struct{ A int }{1}, actual: struct{ A int }{2}, contains: []string{"struct { A int }{A:1}", "struct { A int }{A:2}"}},

		// %#v quotes strings, so whitespace and escapes are visible instead of
		// silently identical-looking.
		{name: "strings differing in whitespace", expected: "a b", actual: "a\tb", contains: []string{`"a b"`, `"a\tb"`}},

		// []byte compares by content, but nil stays distinct from empty:
		// the same rule reflect.DeepEqual applies, kept so Equal does not have
		// two different notions of byte-slice equality.
		{name: "equal byte slices", expected: []byte("ab"), actual: []byte("ab"), wantPass: true},
		{name: "unequal byte slices", expected: []byte("ab"), actual: []byte("ac"), contains: []string{"Not equal:"}},
		{name: "nil vs empty bytes", expected: []byte(nil), actual: []byte{}, contains: []string{"Not equal:"}},
		{name: "bytes vs string", expected: []byte("ab"), actual: "ab", contains: []string{"Not equal:"}},

		// reflect.DeepEqual would report two identical funcs as unequal, so
		// this case is rejected up front rather than reported as a mismatch.
		{name: "func", expected: func() {}, actual: func() {}, contains: []string{"cannot compare func values"}},
		{name: "nil vs func", expected: nil, actual: func() {}, contains: []string{"cannot compare func values"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{name: "rec"}
			got := Equal(r, tt.expected, tt.actual)

			if got != tt.wantPass {
				t.Fatalf("Equal = %v, want %v", got, tt.wantPass)
			}
			if tt.wantPass {
				if len(r.failures) != 0 {
					t.Fatalf("passed but reported: %q", r.failures)
				}
				return
			}
			msg := r.only(t)
			for _, want := range tt.contains {
				if !strings.Contains(msg, want) {
					t.Errorf("failure does not contain %q:\n%s", want, msg)
				}
			}
		})
	}
}

// --- True / False ---

func TestTrue(t *testing.T) {
	r := &recorder{name: "rec"}
	if !True(r, true) {
		t.Fatal("True(true) = false, want true")
	}
	if len(r.failures) != 0 {
		t.Fatalf("True(true) reported: %q", r.failures)
	}

	if True(r, false) {
		t.Fatal("True(false) = true, want false")
	}
	if msg := r.only(t); !strings.Contains(msg, "Should be true") {
		t.Fatalf("failure = %q, want it to mention Should be true", msg)
	}
}

func TestFalse(t *testing.T) {
	r := &recorder{name: "rec"}
	if !False(r, false) {
		t.Fatal("False(false) = false, want true")
	}
	if False(r, true) {
		t.Fatal("False(true) = true, want false")
	}
	if msg := r.only(t); !strings.Contains(msg, "Should be false") {
		t.Fatalf("failure = %q, want it to mention Should be false", msg)
	}
}

// --- Error / NoError ---

func TestNoError(t *testing.T) {
	r := &recorder{name: "rec"}
	if !NoError(r, nil) {
		t.Fatal("NoError(nil) = false, want true")
	}
	if len(r.failures) != 0 {
		t.Fatalf("NoError(nil) reported: %q", r.failures)
	}

	sentinel := errors.New("boom")
	if NoError(r, fmt.Errorf("wrapped: %w", sentinel)) {
		t.Fatal("NoError(err) = true, want false")
	}
	msg := r.only(t)
	if !strings.Contains(msg, "Received unexpected error:") || !strings.Contains(msg, "wrapped: boom") {
		t.Fatalf("failure = %q, want the wrapped error text", msg)
	}
}

func TestError(t *testing.T) {
	r := &recorder{name: "rec"}
	if !Error(r, errors.New("boom")) {
		t.Fatal("Error(err) = false, want true")
	}
	if len(r.failures) != 0 {
		t.Fatalf("Error(err) reported: %q", r.failures)
	}

	if Error(r, nil) {
		t.Fatal("Error(nil) = true, want false")
	}
	if msg := r.only(t); !strings.Contains(msg, "An error is expected but got nil") {
		t.Fatalf("failure = %q", msg)
	}
}

// A typed nil pointer converted to error is NOT a nil interface, so NoError
// reports it and Error accepts it. That is Go's interface rule, not a quirk of
// these assertions: by the time the value arrives the boxing has already
// happened, so the assertion cannot tell what the caller meant.
//
// Pinned because it is a surprise worth having in writing, and because the
// behaviour is the useful one — returning a typed nil from a function is a real
// bug in the code under test, and a helper that quietly smoothed it over would
// hide exactly the failure the test was written to catch.
func TestErrorTypedNilIsNotNil(t *testing.T) {
	var typedNil *myError

	r := &recorder{name: "rec"}
	if NoError(r, typedNil) {
		t.Fatal("NoError(typed nil) = true; a typed nil pointer is not a nil interface")
	}
	if !Error(r, typedNil) {
		t.Fatal("Error(typed nil) = false; a typed nil pointer is not a nil interface")
	}
}

type myError struct{}

func (*myError) Error() string { return "my" }

// --- Contains ---

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        any
		element  any
		wantPass bool
		contains []string
	}{
		{name: "substring", s: "hello world", element: "world", wantPass: true},
		{name: "not a substring", s: "hello", element: "world", contains: []string{"does not contain"}},
		{name: "empty substring", s: "hello", element: "", wantPass: true},

		{name: "slice element", s: []int{1, 2, 3}, element: 2, wantPass: true},
		{name: "slice no element", s: []int{1, 2, 3}, element: 9, contains: []string{"does not contain"}},
		{name: "array element", s: [3]string{"a", "b", "c"}, element: "b", wantPass: true},
		{name: "string slice", s: []string{"a", "b"}, element: "b", wantPass: true},

		{name: "map key", s: map[string]int{"a": 1}, element: "a", wantPass: true},
		{name: "map missing key", s: map[string]int{"a": 1}, element: "b", contains: []string{"does not contain"}},
		// A map looks for keys, not values.
		{name: "map value is not a key", s: map[string]int{"a": 1}, element: 1, contains: []string{"does not contain"}},

		// "not applicable" is reported differently from "not found": the first
		// says the assertion cannot answer the question, the second gives an
		// answer. Collapsing them would send the reader looking for a missing
		// element in a value that never had elements.
		{name: "int is not a collection", s: 1, element: 1, contains: []string{"is not something that can contain"}},
		{name: "nil list", s: nil, element: 1, contains: []string{"is not something that can contain"}},
		{name: "string vs non-string element", s: "abc", element: 1, contains: []string{"is not something that can contain"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{name: "rec"}
			got := Contains(r, tt.s, tt.element)

			if got != tt.wantPass {
				t.Fatalf("Contains = %v, want %v (failures %q)", got, tt.wantPass, r.failures)
			}
			if tt.wantPass {
				if len(r.failures) != 0 {
					t.Fatalf("passed but reported: %q", r.failures)
				}
				return
			}
			msg := r.only(t)
			for _, want := range tt.contains {
				if !strings.Contains(msg, want) {
					t.Errorf("failure does not contain %q:\n%s", want, msg)
				}
			}
		})
	}
}

// A named string element must still work: reflecting on the kind rather than
// asserting to `string` is what makes this pass.
func TestContainsNamedStringElement(t *testing.T) {
	r := &recorder{name: "rec"}
	if !Contains(r, "hello world", NamedString("world")) {
		t.Fatalf("Contains with a named string element failed: %q", r.failures)
	}
}

type NamedString string

// --- Len ---

func TestLen(t *testing.T) {
	tests := []struct {
		name     string
		object   any
		length   int
		wantPass bool
		contains []string
	}{
		{name: "slice", object: []int{1, 2, 3}, length: 3, wantPass: true},
		{name: "slice wrong", object: []int{1, 2, 3}, length: 5, contains: []string{"should have 5 item(s), but has 3"}},
		{name: "empty slice", object: []int{}, length: 0, wantPass: true},
		{name: "nil slice", object: []int(nil), length: 0, wantPass: true},
		{name: "array", object: [3]int{1, 2, 3}, length: 3, wantPass: true},
		{name: "string", object: "abcd", length: 4, wantPass: true},
		{name: "map", object: map[string]int{"a": 1}, length: 1, wantPass: true},
		{name: "channel", object: make(chan int, 2), length: 0, wantPass: true},

		// Types len() does not accept must be a reported failure, not a panic.
		{name: "int", object: 1, length: 1, contains: []string{"int does not have a length"}},
		{name: "nil", object: nil, length: 0, contains: []string{"does not have a length"}},
		{name: "struct", object: struct{ A int }{}, length: 1, contains: []string{"does not have a length"}},
		{name: "bool", object: true, length: 1, contains: []string{"does not have a length"}},
		{name: "pointer to slice", object: &[]int{1, 2}, length: 2, contains: []string{"does not have a length"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{name: "rec"}
			got := Len(r, tt.object, tt.length)

			if got != tt.wantPass {
				t.Fatalf("Len = %v, want %v (failures %q)", got, tt.wantPass, r.failures)
			}
			if tt.wantPass {
				if len(r.failures) != 0 {
					t.Fatalf("passed but reported: %q", r.failures)
				}
				return
			}
			msg := r.only(t)
			for _, want := range tt.contains {
				if !strings.Contains(msg, want) {
					t.Errorf("failure does not contain %q:\n%s", want, msg)
				}
			}
		})
	}
}

// --- messages ---

func TestMessageForms(t *testing.T) {
	tests := []struct {
		name      string
		msgAndArg []any
		want      string // "" means no Messages label at all
	}{
		{name: "none", msgAndArg: nil},
		{name: "one string", msgAndArg: []any{"plain"}, want: "plain"},
		{name: "one non-string", msgAndArg: []any{42}, want: "42"},
		{name: "format and args", msgAndArg: []any{"got %d of %d", 1, 3}, want: "got 1 of 3"},
		{name: "format and no args", msgAndArg: []any{"100%%"}, want: "100%"},

		// testify would panic here. A test helper that panics while reporting
		// a failure hides the failure, so this falls back to printing.
		{name: "non-string format", msgAndArg: []any{42, "x"}, want: "42 x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &recorder{name: "rec"}
			True(r, false, tt.msgAndArg...)
			msg := r.only(t)

			if tt.want == "" {
				if strings.Contains(msg, "Messages:") {
					t.Fatalf("no message expected, got:\n%s", msg)
				}
				return
			}
			if !strings.Contains(msg, "Messages:") || !strings.Contains(msg, tt.want) {
				t.Fatalf("failure = %q, want Messages to contain %q", msg, tt.want)
			}
		})
	}
}

// Every assertion marks itself as a helper, so a failure points at the caller
// rather than into this package. Testing that needs a real *testing.T and a
// real failure, which is what TestHelperChain in output_test.go does.

func TestFailureLabels(t *testing.T) {
	r := &recorder{name: "TestSomething"}
	Equal(r, 1, 2, "why")
	msg := r.only(t)

	for _, label := range []string{"Error:", "Test:", "Messages:"} {
		if !strings.Contains(msg, label) {
			t.Errorf("failure block is missing %q:\n%s", label, msg)
		}
	}
	if !strings.Contains(msg, "TestSomething") {
		t.Errorf("failure block does not name the test:\n%s", msg)
	}
	// The leading newline is what lifts the block out from behind the
	// "file.go:12: " prefix that the testing package adds.
	if !strings.HasPrefix(msg, "\n") {
		t.Errorf("failure should start with a newline, got %q", msg)
	}
}

// A TestingT without Name() must still work: the Test label is optional.
func TestFailureWithoutName(t *testing.T) {
	r := &bareRecorder{}
	Equal(r, 1, 2)
	if len(r.failures) != 1 {
		t.Fatalf("failures = %d, want 1", len(r.failures))
	}
	if strings.Contains(r.failures[0], "Test:") {
		t.Errorf("no Name() means no Test label:\n%s", r.failures[0])
	}
}

type bareRecorder struct{ failures []string }

func (r *bareRecorder) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// --- output shape ---

func TestLabeledOutputAlignment(t *testing.T) {
	got := labeledOutput(
		labeledContent{label: "Error", content: "Not equal:\nexpected: 1\nactual  : 2"},
		labeledContent{label: "Test", content: "TestX"},
		labeledContent{label: "Messages", content: "why"},
	)

	// Labels pad to the longest ("Messages", 8), and continuation lines indent
	// to the content column: tab + longest+1 spaces + tab.
	want := "\tError:   \tNot equal:\n" +
		"\t         \texpected: 1\n" +
		"\t         \tactual  : 2\n" +
		"\tTest:    \tTestX\n" +
		"\tMessages:\twhy\n"

	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestIndentLines(t *testing.T) {
	if got := indentLines("one line", 8); got != "one line" {
		t.Fatalf("single line changed: %q", got)
	}
	got := indentLines("a\nb", 4)
	want := "a\n\t     \tb"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFormatValues(t *testing.T) {
	// Same type: no type names, kept readable.
	exp, act := formatValues(1, 2)
	if exp != "1" || act != "2" {
		t.Fatalf("formatValues(1,2) = %q, %q", exp, act)
	}
	// Different types: the type is the surprise, so it is named. %#v alone
	// prints 1 for both of these.
	exp, act = formatValues(1, int64(1))
	if exp != "int(1)" || act != "int64(1)" {
		t.Fatalf("formatValues(1,int64(1)) = %q, %q", exp, act)
	}
	// Same type, composite: %#v carries the type itself.
	exp, act = formatValues(struct{ A int }{1}, struct{ A int }{2})
	if exp != "struct { A int }{A:1}" || act != "struct { A int }{A:2}" {
		t.Fatalf("formatValues(structs) = %q, %q", exp, act)
	}
	// nil has no type to name.
	if exp, act = formatValues(nil, 1); exp != "nil" {
		t.Fatalf("formatValues(nil,1) = %q, %q", exp, act)
	}
}

// A single value must not be allowed to grow past what the testing package will
// print: it drops any output line over bufio.MaxScanTokenSize, so an unbounded
// value would cost the whole message instead of just being long.
func TestFormatBounded(t *testing.T) {
	short := formatBounded("%#v", 1)
	if short != "1" {
		t.Fatalf("short value changed: %q", short)
	}

	huge := strings.Repeat("x", maxPrintedValue+1000)
	got := formatBounded("%#v", huge)
	if len(got) >= len(huge) {
		t.Fatalf("not bounded: %d >= %d", len(got), len(huge))
	}
	if !strings.HasSuffix(got, "<... truncated>") {
		t.Fatalf("truncation is not marked: %q", got[len(got)-30:])
	}

	// And the whole failure message has to stay inside the scanner's limit,
	// which is the point of the bound.
	r := &recorder{name: "rec"}
	Equal(r, huge, "other")
	msg := r.only(t)
	for _, line := range strings.Split(msg, "\n") {
		if len(line) > bufio.MaxScanTokenSize {
			t.Fatalf("a failure line is %d bytes, over the %d limit: the testing package would drop it",
				len(line), bufio.MaxScanTokenSize)
		}
	}
}

// --- helpers ---

func TestObjectsAreEqual(t *testing.T) {
	tests := []struct {
		name     string
		expected any
		actual   any
		want     bool
	}{
		{name: "equal ints", expected: 1, actual: 1, want: true},
		{name: "unequal ints", expected: 1, actual: 2},
		{name: "both nil", expected: nil, actual: nil, want: true},
		{name: "nil vs value", expected: nil, actual: 1},
		{name: "equal bytes", expected: []byte("ab"), actual: []byte("ab"), want: true},
		{name: "nil vs empty bytes", expected: []byte(nil), actual: []byte{}},
		{name: "both nil bytes", expected: []byte(nil), actual: []byte(nil), want: true},
		{name: "bytes vs string", expected: []byte("ab"), actual: "ab"},
		{name: "equal nested", expected: map[string][]int{"a": {1}}, actual: map[string][]int{"a": {1}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := objectsAreEqual(tt.expected, tt.actual); got != tt.want {
				t.Fatalf("objectsAreEqual = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLen(t *testing.T) {
	if l, ok := getLen([]int{1, 2}); !ok || l != 2 {
		t.Fatalf("getLen(slice) = %d, %v", l, ok)
	}
	if l, ok := getLen("abc"); !ok || l != 3 {
		t.Fatalf("getLen(string) = %d, %v", l, ok)
	}
	if _, ok := getLen(1); ok {
		t.Fatal("getLen(int) reported applicable")
	}
	if _, ok := getLen(nil); ok {
		t.Fatal("getLen(nil) reported applicable")
	}
}

func TestContainsElement(t *testing.T) {
	tests := []struct {
		name      string
		list      any
		element   any
		wantOK    bool
		wantFound bool
	}{
		{name: "string found", list: "abc", element: "b", wantOK: true, wantFound: true},
		{name: "string not found", list: "abc", element: "z", wantOK: true},
		{name: "slice found", list: []int{1, 2}, element: 2, wantOK: true, wantFound: true},
		{name: "map found", list: map[string]int{"a": 1}, element: "a", wantOK: true, wantFound: true},
		{name: "map not found", list: map[string]int{"a": 1}, element: "z", wantOK: true},
		{name: "nil list", list: nil, element: 1},
		{name: "int list", list: 1, element: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, found := containsElement(tt.list, tt.element)
			if ok != tt.wantOK || found != tt.wantFound {
				t.Fatalf("containsElement = %v, %v; want %v, %v", ok, found, tt.wantOK, tt.wantFound)
			}
		})
	}
}

// --- interface surface ---

// TestingT must stay one method wide: that is what lets any type stand in for
// *testing.T, including the recorder above. Widening it (adding Helper, for
// instance) would break every custom TestingT, so this pins it.
func TestTestingTSurface(t *testing.T) {
	typ := reflect.TypeOf((*TestingT)(nil)).Elem()

	if got := typ.NumMethod(); got != 1 {
		methods := make([]string, typ.NumMethod())
		for i := range typ.NumMethod() {
			methods[i] = typ.Method(i).Name
		}
		t.Fatalf("TestingT has %d methods (%v), want exactly 1: widening it forces every custom TestingT to grow too", got, methods)
	}
	if name := typ.Method(0).Name; name != "Errorf" {
		t.Fatalf("TestingT method is %q, want Errorf", name)
	}
}

// The real testing.T and testing.B must satisfy TestingT, or none of this is
// usable.
func TestTestingTIsSatisfied(t *testing.T) {
	var _ TestingT = (*testing.T)(nil)
	var _ TestingT = (*testing.B)(nil)
}
