package assert

import (
	"bufio"
	"fmt"
	"reflect"
	"strings"
)

// fail is the single failure exit: the shape of a failure message is defined
// here and nowhere else.
//
// It always returns false, so an assertion that has nothing left to check can
// `return fail(t, ...)` and satisfy its bool result in one line — which is the
// only reason the return type is bool at all.
//
// The Helper call matters here as much as in each assertion: Helper marks the
// *calling* function as skippable, so a failure reports the first frame that
// did not ask to be skipped. Every assertion calls it too, otherwise the
// reported line would be the assertion's own line in this file.
func fail(t TestingT, failureMessage string, msgAndArgs ...any) bool {
	if h, ok := t.(tHelper); ok {
		h.Helper()
	}

	content := []labeledContent{{label: "Error", content: failureMessage}}
	if n, ok := t.(interface{ Name() string }); ok {
		content = append(content, labeledContent{label: "Test", content: n.Name()})
	}
	if msg := messageFromMsgAndArgs(msgAndArgs...); msg != "" {
		content = append(content, labeledContent{label: "Messages", content: msg})
	}

	t.Errorf("\n%s", labeledOutput(content...))
	return false
}

// messageFromMsgAndArgs interprets the optional trailing arguments:
//
//	none            → ""
//	one string      → as is
//	one non-string  → %+v
//	format, args... → fmt.Sprintf(format, args...)
//
// The last form reuses Go's own printf convention instead of giving every
// assertion an `f` variant. Its cost is that the format string gets no
// compile-time check, and that the arguments are positional by hand — the
// tradeoff described in the package doc.
//
// Where testify asserts that the first argument is a string (panicking
// otherwise), this falls back to printing the whole argument list. A test
// helper that panics while reporting a failure hides the failure.
func messageFromMsgAndArgs(msgAndArgs ...any) string {
	switch len(msgAndArgs) {
	case 0:
		return ""
	case 1:
		if s, ok := msgAndArgs[0].(string); ok {
			return s
		}
		return fmt.Sprintf("%+v", msgAndArgs[0])
	default:
		if format, ok := msgAndArgs[0].(string); ok {
			return fmt.Sprintf(format, msgAndArgs[1:]...)
		}
		return fmt.Sprintf("%+v", msgAndArgs)
	}
}

// labeledContent is one label/value pair in the failure block.
type labeledContent struct {
	label   string
	content string
}

// labeledOutput renders the failure as a block of aligned labels:
//
//	Error:     Not equal:
//	           expected: 1
//	           actual  : 2
//	Test:      TestSomething
//	Messages:  ...
//
// Continuation lines are indented to the content column so that a multi-line
// value stays visually attached to its label. Every line starts with a tab,
// which undoes the indentation the testing package adds to a failure message.
// The leading newline in fail's format string is what gets the block out from
// behind the "file.go:12: " prefix.
func labeledOutput(content ...labeledContent) string {
	longest := 0
	for _, c := range content {
		if n := len(c.label); n > longest {
			longest = n
		}
	}

	var sb strings.Builder
	for _, c := range content {
		sb.WriteString("\t")
		sb.WriteString(c.label)
		sb.WriteString(":")
		sb.WriteString(strings.Repeat(" ", longest-len(c.label)))
		sb.WriteString("\t")
		sb.WriteString(indentLines(c.content, longest))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// indentLines indents every line but the first so that continuation lines line
// up under the start of the first one.
func indentLines(content string, longestLabel int) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 1 {
		return content
	}

	var sb strings.Builder
	sb.WriteString(lines[0])
	pad := "\n\t" + strings.Repeat(" ", longestLabel+1) + "\t"
	for _, line := range lines[1:] {
		sb.WriteString(pad)
		sb.WriteString(line)
	}
	return sb.String()
}

// formatValues renders two values for a comparison failure.
//
// The verb is %#v, which prints a struct together with its type and field names
// and quotes strings — so a difference in whitespace is visible rather than
// invisible. An explicit type name is added only when the two sides disagree:
// %#v alone prints '1' for both int(1) and int64(1), and that pairing is
// otherwise baffling.
func formatValues(expected, actual any) (string, string) {
	if reflect.TypeOf(expected) != reflect.TypeOf(actual) {
		return typedValue(expected), typedValue(actual)
	}
	return formatBounded("%#v", expected), formatBounded("%#v", actual)
}

// typedValue renders a value with its type, for when the type is the surprise.
func typedValue(v any) string {
	if v == nil {
		return "nil"
	}
	return fmt.Sprintf("%T(%s)", v, formatBounded("%#v", v))
}

// maxPrintedValue bounds how much of a single value a message may contain.
//
// This is not a readability choice. The testing package scans its own output
// with a bufio.Scanner, whose default maximum token size is 64 KiB, and a line
// longer than that is dropped rather than shown — so an unbounded value would
// not yield an ugly message, it would yield no message at all. Half the budget
// per value leaves room for the surrounding text and a second value.
//
// Real failures are orders of magnitude below this. The cap exists so that an
// enormous value fails legibly instead of silently.
const maxPrintedValue = bufio.MaxScanTokenSize/2 - 100

// formatBounded renders v with the given verb, cutting the result off at
// maxPrintedValue.
func formatBounded(verb string, v any) string {
	s := fmt.Sprintf(verb, v)
	if len(s) <= maxPrintedValue {
		return s
	}
	return s[:maxPrintedValue] + "<... truncated>"
}
