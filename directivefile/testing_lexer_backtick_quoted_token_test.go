package directivefile

import (
	"testing"
)

func TestTokenizeBacktickStringGoodCases(t *testing.T) {
	configFilePlaceholder := "input.conf"
	tests := []struct {
		name  string
		input []rune
		want  string
	}{
		{name: "empty", input: []rune{'`', '`'}, want: ""},
		{name: "backslash", input: []rune{'`', '\\', '`'}, want: "\\"},
		{name: "escape-a", input: []rune{'`', '\\', 'a', '`'}, want: "\\a"},
		{name: "escape-b", input: []rune{'`', '\\', 'b', '`'}, want: "\\b"},
		{name: "escape-n", input: []rune{'`', '\\', 'n', '`'}, want: "\\n"},
		{name: "escape-v", input: []rune{'`', '\\', 'v', '`'}, want: "\\v"},
		{name: "escape-t", input: []rune{'`', '\\', 't', '`'}, want: "\\t"},
		{name: "carriage-return", input: []rune{'`', '\r', '`'}, want: "\r"},
		{name: "double-backslash", input: []rune{'`', '\\', '\\', '`'}, want: "\\\\"},
		{name: "double-quote-single-quote", input: []rune{'`', '"', '\\', '"', '\'', '`'}, want: `"\"'`},
		{name: "double-quotes", input: []rune{'`', '"', '"', '`'}, want: `""`},
		{name: "many-double-quotes", input: []rune{'`', '"', '"', '"', '"', '"', '"', '"', '"', '`'}, want: `""""""""`},
		{name: "spaces", input: []rune{'`', ' ', ' ', ' ', ' ', ' ', ' ', ' ', '`'}, want: "       "},
		{name: "spaces-and-words", input: []rune{'`', ' ', ' ', ' ', 'a', 'a', 'a', ' ', 'b', 'b', 'b', ' ', 'c', 'c', 'c', ' ', '`'}, want: "   aaa bbb ccc "},
		{name: "left-brace", input: []rune{'`', '{', '`'}, want: "{"},
		{name: "right-brace", input: []rune{'`', '}', '`'}, want: "}"},
		{name: "braces", input: []rune{'`', '{', '}', '`'}, want: "{}"},
		{
			name:  "comment",
			input: []rune{'`', '#', ' ', 's', 'o', 'm', 'e', ' ', 'c', 'o', 'o', 'l', ' ', 't', 'e', 'x', 't', '`'},
			want:  "# some cool text",
		},
		{name: "unicode-4-digit", input: []rune{'`', '\\', 'u', '4', 'F', '6', '0', '`'}, want: "\\u4F60"},
		{name: "unicode-8-digit", input: []rune{'`', '\\', 'U', '0', '0', '0', '1', 'F', '6', '0', '0', '`'}, want: "\\U0001F600"},
		{name: "windows-path", input: []rune{'`', 'C', ':', '\\', 'p', 'a', 't', 'h', '`'}, want: `C:\path`},
		{name: "quoted-escape-like-text", input: []rune{'`', '"', 'a', '\\', 'n', 'b', '`'}, want: `"a\nb`},
		{name: "quoted-escape-like-comment", input: []rune{'`', '"', 'a', '\\', 'n', '#', 'b', 'b', 'b', '`'}, want: `"a\n#bbb`},
		{name: "unc-path", input: []rune{'`', '\\', '\\', 's', 'e', 'r', 'v', 'e', 'r', '\\', 's', 'h', 'a', 'r', 'e', '`'}, want: `\\server\share`},
		{
			name:  "regex",
			input: []rune{'`', '\\', 'd', '+', '\\', '.', '\\', 'd', '+', '`'},
			want:  `\d+\.\d+`,
		},
		{name: "backslash-escapes", input: []rune{'`', '\\', 'a', '\\', 'b', '\\', 'f', '\\', 'n', '\\', 'r', '\\', 't', '\\', 'v', '\\', '\\', '\\', '\'', '`'}, want: `\a\b\f\n\r\t\v\\\'`},
		{name: "dollar", input: []rune{'`', '$', '`'}, want: "$"},
		{name: "braced-variable", input: []rune{'`', '{', '$', 'H', 'O', 'M', 'E', '}', '`'}, want: "{$HOME}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok, err := Tokenize([]byte(string(tt.input)), configFilePlaceholder)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(tok) != 2 {
				t.Fatalf("expected 2 token, got %d", len(tok))
			}

			if tok[0].Type != BacktickQuotedToken {
				t.Fatalf("expected BacktickQuotedString, got %v", tok[0].Type)
			}

			if tok[0].Text != tt.want {
				t.Errorf("expected text %q, got %q", tt.want, tok[0].Text)
			}
		})
	}

	t.Run("backtick_normal_1", func(t *testing.T) {
		const configpath = "testsdata/lexer_token/goodcase/3-backtick-quoted-1.conf"
		want := `aaa
bbb`
		tokens, err := Tokenize(readTestdata(t, configpath), configpath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := valuesOf(tokens)[0]
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("backtick_normal_2", func(t *testing.T) {
		const configpath = "testsdata/lexer_token/goodcase/3-backtick-quoted-2.conf"
		want := `aaa


bbb
`
		tokens, err := Tokenize(readTestdata(t, configpath), configpath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := valuesOf(tokens)[0]
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("backtick_normal_3", func(t *testing.T) {
		const configpath = "testsdata/lexer_token/goodcase/3-backtick-quoted-3.conf"
		want := `aaa
# some cool 
`
		tokens, err := Tokenize(readTestdata(t, configpath), configpath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got := valuesOf(tokens)[0]
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})

	t.Run("backtick_normal_4", func(t *testing.T) {
		const configpath = "testsdata/lexer_token/goodcase/3-backtick-quoted-4.conf"
		want := `
aaa


bbb
`
		tokens, err := Tokenize(readTestdata(t, configpath), configpath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got := tokens[1].Text
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}

		x := tokens[2]
		if x.Text != "x" {
			t.Fatalf("expected token text %q, got %q", "x", x.Text)
		}

		wantPos := Position{
			Filename: configpath,
			Line:     7,
			Column:   3,
		}
		if x.Position != wantPos {
			t.Errorf("expected position %+v, got %+v", wantPos, x.Position)
		}
	})
}

func TestTokenizeBacktickStringBadCases(t *testing.T) {
	configFilePlaceholder := "input.conf"
	tests := []struct {
		name  string
		input []rune
	}{
		{
			name:  "unterminated_empty",
			input: []rune{'`'},
		},
		{
			name:  "unterminated",
			input: []rune{'`', 'a', 'a', 'a'},
		},
		{
			name:  "backtick_followed_by_a",
			input: []rune{'`', 'a', 'a', 'a', '`', 'a'},
		},
		{
			name:  "backtick_followed_by_double_quote",
			input: []rune{'`', 'a', 'a', 'a', '`', '"'},
		},
		{
			name:  "double_backtick",
			input: []rune{'`', 'a', 'a', 'a', '`', '`'},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Tokenize([]byte(string(tt.input)), configFilePlaceholder)
			if err == nil {
				t.Fatal("unexpected error got nil")
			}
		})
	}

	t.Run("backtick_badcase_1", func(t *testing.T) {
		const path = "testsdata/lexer_token/badcase/3-backtick-quoted-1.conf"
		_, err := Tokenize(readTestdata(t, path), path)
		if err == nil {
			t.Fatal("unexpected error got nil")
		}
	})

	t.Run("backtick_badcase_2", func(t *testing.T) {
		const path = "testsdata/lexer_token/badcase/3-backtick-quoted-2.conf"
		_, err := Tokenize(readTestdata(t, path), path)
		if err == nil {
			t.Fatal("unexpected error got nil")
		}
	})

	t.Run("backtick_badcase_3", func(t *testing.T) {
		const path = "testsdata/lexer_token/badcase/3-backtick-quoted-3.conf"
		_, err := Tokenize(readTestdata(t, path), path)
		if err == nil {
			t.Fatal("unexpected error got nil")
		}
	})
}
