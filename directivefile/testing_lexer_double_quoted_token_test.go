package directivefile

import (
	"strings"
	"testing"
)

// A double-quoted string is an interpreted string: it stops at the next
// unescaped double quote and the escape sequences inside it are decoded, with
// `\u` / `\U` naming a code point and every other `\X` preserved literally.
// It cannot span lines.
//
// test data testsdata/lexer_token/goodcase/2-double-quoted-1.conf
func TestTokenizeDoubleQuotedStringGoodCases(t *testing.T) {
	configFilePlaceholder := "config.conf"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty",
			input: `""`,
			want:  "",
		},
		{
			name:  "have a space",
			input: `" "`,
			want:  " ",
		},
		{
			name:  "backtick and single quote",
			input: string([]rune{'"', '`', '\'', '"'}),
			want:  "`'",
		},
		{
			name:  "ASCII characters",
			input: `"skrik2"`,
			want:  "skrik2",
		},
		{
			name:  "Latin characters",
			input: `"ä"`,
			want:  "ä",
		},
		{
			name:  "CJK characters",
			input: `"击缶而歌"`,
			want:  "击缶而歌",
		},
		{
			name:  "unknown escape sequence",
			input: `"hell\o"`,
			want:  `hell\o`,
		},
		{
			name:  "consecutive backslash escapes",
			input: `"\\\\\\"`,
			want:  `\\\`,
		},
		{
			name:  "Emoji characters",
			input: `"✌🏻🙄✌🏻"`,
			want:  "✌🏻🙄✌🏻",
		},
		{
			name: "escape sequences",
			input: string([]rune{
				'"',
				'\\', 'a',
				'\\', 'b',
				'\\', 'f',
				'\\', 'n',
				'\\', 'r',
				'\\', 't',
				'\\', 'v',
				'\\', '\\',
				'\\', '"',
				'"',
			}),
			want: "\a\b\f\n\r\t\v\\\"",
		},
		{
			name:  "Unicode escapes",
			input: `"\u4F60\u597D \U0001F600"`,
			want:  "你好 😀",
		},
		{
			name:  "Unicode escape U+0000",
			input: `"\u0000"`,
			want:  string(rune(0)),
		},
		{
			name:  "Unicode escape U+10FFFF",
			input: `"\U0010FFFF"`,
			want:  string(rune(0x10FFFF)),
		},
		{
			name:  "lowercase hexadecimal digits",
			input: `"\uabcd"`,
			want:  "ꯍ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokens, err := Tokenize([]byte(tc.input), configFilePlaceholder)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// The inline inputs have no trailing newline, so Tokenize appends the
			// NewLine it synthesises at EOF — compare values only.
			got := valuesOf(tokens)
			if len(got) != 1 {
				t.Fatalf("expected 1 value token, got %d (%q)", len(got), got)
			}
			if got[0] != tc.want {
				t.Fatalf("expected token text %q, got %q", tc.want, got[0])
			}
		})
	}

	t.Run("only one double-quoted-string token in file 1", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer_token/goodcase/2-double-quoted-2.conf"
		_, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatal("expected nil, got error")
		}
	})

	t.Run("only one double-quoted-string token in file 2", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer_token/goodcase/2-double-quoted-3.conf"
		_, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatal("expected nil, got error")
		}
	})

	t.Run("only one double-quoted-string token in file 3", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer_token/goodcase/2-double-quoted-4.conf"
		_, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatal("expected nil, got error")
		}
	})
}

// test data testsdata/lexer_token/badcase/2-double-quoted.conf
// test data testsdata/lexer_token/badcase/2-double-quoted-unicode.conf
func TestTokenizeDoubleQuotedStringBadCases(t *testing.T) {
	configFilePlaceholder := "config.conf"
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name:    "non-separator after closing quote",
			input:   `"aaa"b`,
			wantErr: "unexpected character after double-quoted string",
		},
		{
			name:    "escape at EOF",
			input:   `"aaaa\`,
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "string at EOF",
			input:   `"aaaa`,
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "incomplete Unicode escape at EOF",
			input:   "\"num:\\u",
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "incomplete Unicode escape at EOF",
			input:   "\"num:\\U",
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "incomplete Unicode escape at EOF",
			input:   "\"num:\\u",
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "incomplete Unicode escape",
			input:   "\"num:\\u000",
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "invalid Unicode escape sequence",
			input:   `"num:\uZZZZ`,
			wantErr: "invalid Unicode escape sequence",
		},
		{
			name:    "Unicode escape followed by EOF",
			input:   "\"num:\\u0000",
			wantErr: "unterminated double-quoted string",
		},
		{
			name:    "Unicode code point out of range",
			input:   `"\U00110000"`,
			wantErr: "invalid Unicode escape sequence",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Tokenize([]byte(tc.input), configFilePlaceholder)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err)
			}
		})
	}

	t.Run("double-quoted string unterminated", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer_token/badcase/2-double-quoted-unterminated-1.conf"
		const wantErr = "unterminated double-quoted string"
		_, err := Tokenize(readTestdata(t, path), path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("expected error containing %q, got %q", wantErr, err)
		}
	})
}
