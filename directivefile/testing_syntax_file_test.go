package directivefile

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseFileGoodCases drives the file goodcases through parse() and asserts
// the whole tree, position included.
//
// 1-simple-block.conf and 2-nested-block.conf are driven by
// TestParseCommandGoodCases instead; 3-backtick.conf and 4-comment.conf are not
// driven yet.
func TestParseFileGoodCases(t *testing.T) {
	t.Run("5-brace-in-quote.conf", func(t *testing.T) {
		const path = "testsdata/syntax/file/goodcases/5-brace-in-quote.conf"

		got, err := parse(path)
		if err != nil {
			t.Fatalf("parse() error = %v", err)
		}

		// A brace is structural only when it is a token of its own, so a `{` or
		// `}` inside quotes (either quoting kind) is an ordinary argument and
		// never opens or closes a block.
		want := File{Commands: []Command{{
			Directive: "command",
			Position:  Position{Filename: path, Line: 1, Column: 1},
			SubCommands: []Command{
				{
					Directive: "arg",
					Args:      []string{"{"},
					Position:  Position{Filename: path, Line: 2, Column: 5},
				},
				{
					Directive: "arg",
					Args:      []string{"}"},
					Position:  Position{Filename: path, Line: 3, Column: 5},
				},
				{
					Directive: "arg",
					Args:      []string{"{", "}"},
					Position:  Position{Filename: path, Line: 4, Column: 5},
				},
			},
		}}}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parse():\n got: %#v\nwant: %#v", got, want)
		}
	})

	// A dot is a name character, not punctuation: `dns.forwarder` is ONE token
	// and a valid directive — at the top level and inside a block alike, and with
	// as many segments as the module id has.
	t.Run("6-dotted-directive.conf", func(t *testing.T) {
		const path = "testsdata/syntax/file/goodcases/6-dotted-directive.conf"

		got, err := parse(path)
		if err != nil {
			t.Fatalf("parse() error = %v", err)
		}

		want := File{Commands: []Command{
			{
				Directive: "dns.forwarder",
				Position:  Position{Filename: path, Line: 12, Column: 1},
				SubCommands: []Command{{
					Directive: "upstream",
					Args:      []string{"1.1.1.1"},
					Position:  Position{Filename: path, Line: 13, Column: 5},
				}},
			},
			{
				Directive: "endpoint",
				Position:  Position{Filename: path, Line: 16, Column: 1},
				SubCommands: []Command{{
					Directive: "endpoint.http.proxy",
					Position:  Position{Filename: path, Line: 17, Column: 5},
					SubCommands: []Command{{
						Directive: "to",
						Args:      []string{"127.0.0.1:8080"},
						Position:  Position{Filename: path, Line: 18, Column: 9},
					}},
				}},
			},
		}}

		if !reflect.DeepEqual(got, want) {
			t.Fatalf("parse():\n got: %#v\nwant: %#v", got, want)
		}
	})
}

// TestParseFileBadCases drives every file badcase through parse().
//
// The expected error is spelled out in full — message AND position — because
// where an error is reported is part of the contract. The path prefix is part
// of it too: it proves the error names the right file.
//
// The files are numbered by order, not by rule: 1-3 reject an ill-formed block
// (the three structural criteria in wiki/spec.md), 4-5 reject a brace that never
// became a structural token, 6 rejects a depth overflow, 7-10 reject a malformed
// dotted name.
func TestParseFileBadCases(t *testing.T) {
	// The `{` sits on a line of its own, so `dns` ends at the newline and the
	// `{` starts a command — which fails the directive shape rule. This is
	// criterion 3: a block is the LAST ARGUMENT of its directive.
	t.Run("1-block-open-next-line", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/1-block-open-next-line.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":3:1: syntax: invalid directive: {"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// Criterion 1: `{` must be followed by a newline. The error points at the
	// token that should have been that newline — here the `}` of `{ }`, which is
	// why the column is 15 rather than the position of the `{` itself.
	t.Run("2-block-open-same-line", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/2-block-open-same-line.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":11:15: syntax: expected newline after '{' (block must start on a new line)"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// Criterion 2: a block must hold at least one command. Reported at the `{`
	// of the inner block, which is the empty one.
	t.Run("3-empty-block", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/3-empty-block.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":9:13: syntax: empty block is not allowed"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// `{ "key1:val1" }` is all on one line inside a command, so the `{` is
	// followed by a value rather than a newline.
	t.Run("4-brace-in-one-line-1", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/4-brace-in-one-line-1.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":2:22: syntax: expected newline after '{' (block must start on a new line)"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// The same fault with no closing brace at all, so this one cannot be
	// mistaken for a block that merely starts on the wrong line.
	t.Run("5-brace-in-one-line-2", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/5-brace-in-one-line-2.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":1:7: syntax: expected newline after '{' (block must start on a new line)"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// The position is the interesting half here. maxCommandDepth = 64 is counted
	// in parseSubCommands, which is entered once per `{`, so the FIRST refused
	// brace is the 64th one — line 80, the `{` of `a63 {`. Pinning that line
	// pins the boundary from both sides: a limit of 63 would report line 79, a
	// limit of 65 would report nothing at all.
	t.Run("6-command-nesting-too-deep", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/6-command-nesting-too-deep.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":80:5: syntax: command nesting depth exceeds 64"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// The dot is a name character only AFTER the first one, so a bare namespace
	// tail is rejected instead of being read as a nested name. That is what
	// keeps the rule a charset rather than "anything goes".
	t.Run("7-dotted-directive-leading-dot", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/7-dotted-directive-leading-dot.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":8:1: syntax: invalid directive: .dns"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// A trailing dot leaves the last segment empty. A namespace is built by
	// joining a parent onto a child, so the join rule cannot produce one — it is
	// always a typo.
	t.Run("8-dotted-directive-trailing-dot", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/8-dotted-directive-trailing-dot.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":9:1: syntax: invalid directive: dns."
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// An empty segment in the middle is the same fault as a trailing dot.
	t.Run("9-dotted-directive-empty-segment", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/9-dotted-directive-empty-segment.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":7:1: syntax: invalid directive: dns..forwarder"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})

	// Each segment must itself be an identifier, so it may not start with a
	// digit — or, by the same rule, with `-`. The characters remain legal LATER
	// in a segment: `dns.2x` is a typo, `dns.srv2` is a name.
	t.Run("10-dotted-directive-digit-segment", func(t *testing.T) {
		const path = "testsdata/syntax/file/badcases/10-dotted-directive-digit-segment.conf"

		_, err := parse(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		wantErr := path + ":11:1: syntax: invalid directive: dns.2x"
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})
}
