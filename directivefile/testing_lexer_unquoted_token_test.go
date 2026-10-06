package directivefile

import (
	"strings"
	"testing"
)

// wantTok is one expected token: its type, its exact Text, and its Position.
// Every field is compared always, so spell out the NewLine's Text as "".
type wantTok struct {
	typ  TokenType
	text string
	line int
	col  int
}

// checkUnquotedFile tokenizes one 1-unquoted-string-* fixture and compares the
// whole token stream, Position included.
func checkUnquotedFile(t *testing.T, file string, want []wantTok) {
	t.Helper()

	path := "testsdata/lexer_token/goodcase/" + file
	tokens, err := Tokenize(readTestdata(t, path), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != len(want) {
		t.Fatalf("token count: want %d, got %d\n got: %s", len(want), len(tokens), describeTokens(tokens))
	}
	for i, w := range want {
		g := tokens[i]
		if g.Type != w.typ || g.Text != w.text ||
			g.Position.Line != w.line || g.Position.Column != w.col {
			t.Errorf("token %d: want %s(%q)@%d:%d, got %s(%q)@%d:%d",
				i, w.typ, w.text, w.line, w.col,
				g.Type, g.Text, g.Position.Line, g.Position.Column)
		}
	}
}

// A bare token is a raw string that may not contain whitespace: it runs from its
// first rune to the next Token separator, LF or EOF, and nothing inside it is
// escaped, decoded or transformed — its Text is the source verbatim.
//
// A quote is therefore an ordinary character *inside* a bare token. Only a quote
// in the first position starts a quoted string, which is why `aaa"` is one bare
// token while `aaa "bbb"` is two tokens.
func TestTokenizeUnquotedString(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-minimal.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			{NewLine, "", 1, 4},
		})
	})

	t.Run("two-on-one-line", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-two-on-one-line.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			{UnquotedToken, "bbb", 1, 5},
			{NewLine, "", 1, 8},
		})
	})

	t.Run("trailing-comment", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-trailing-comment.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			// `# some cool text` starts at 1:5 and runs to EOF, so it is a
			// comment, not part of the token.
			{NewLine, "", 1, 21},
		})
	})

	t.Run("comment-no-space", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-comment-no-space.conf", []wantTok{
			{NewLine, "", 1, 31},
			{UnquotedToken, "aaa", 2, 1},
			// `#blah blah blah` starts at 2:5 — a comment needs only to be at
			// the START of a token, not preceded by a space.
			{NewLine, "", 2, 20},
		})
	})

	t.Run("before-double-quoted", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-before-double-quoted.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			{DoubleQuotedToken, "bbb", 1, 5},
			{NewLine, "", 1, 10},
		})
	})

	t.Run("before-backtick", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-before-backtick.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			{BacktickQuotedToken, "bbb", 1, 5},
			{NewLine, "", 1, 10},
		})
	})

	t.Run("before-brace", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-before-brace.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			// A brace is an ordinary character: `{` alone is just a token whose
			// Text is "{".
			{UnquotedToken, "{", 1, 5},
			{NewLine, "", 1, 6},
		})
	})

	t.Run("before-brace-pair", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-before-brace-pair.conf", []wantTok{
			{UnquotedToken, "aaa", 1, 1},
			// `{}` with no space is ONE token, so it can never be a block.
			{UnquotedToken, "{}", 1, 5},
			{NewLine, "", 1, 7},
		})
	})

	t.Run("position", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-position.conf", []wantTok{
			{NewLine, "", 1, 28},
			{NewLine, "", 2, 8},
			// 4 spaces of indentation: the token starts at column 5.
			{UnquotedToken, "aaa", 3, 5},
			{NewLine, "", 3, 8},
		})
	})

	t.Run("windows-path", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-windows-path.conf", []wantTok{
			{UnquotedToken, `C:\Users\me`, 1, 1},
			{NewLine, "", 1, 12},
		})
	})

	t.Run("regexp", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-regexp.conf", []wantTok{
			{UnquotedToken, `\d+\.\d+`, 1, 1},
			{NewLine, "", 1, 9},
		})
	})

	t.Run("unc-path", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-unc-path.conf", []wantTok{
			{UnquotedToken, `\\server\share`, 1, 1},
			{NewLine, "", 1, 15},
		})
	})

	t.Run("two-backslashes", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-two-backslashes.conf", []wantTok{
			// A token made only of backslashes is a perfectly ordinary value:
			// `\` is not special inside a bare token.
			{UnquotedToken, `\\`, 1, 1},
			{NewLine, "", 1, 3},
		})
	})

	t.Run("fourteen-backslashes", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-fourteen-backslashes.conf", []wantTok{
			{UnquotedToken, strings.Repeat(`\`, 14), 1, 1},
			{NewLine, "", 1, 15},
		})
	})

	t.Run("emoji", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-emoji.conf", []wantTok{
			{UnquotedToken, "✌🏻🙄✌🏻", 1, 1},
			{NewLine, "", 1, 6},
		})
	})

	t.Run("cjk", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-cjk.conf", []wantTok{
			{UnquotedToken, "但为君故，沉吟至今", 1, 1},
			{NewLine, "", 1, 10},
		})
	})

	t.Run("multibyte-mixed", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-multibyte-mixed.conf", []wantTok{
			{NewLine, "", 1, 15},
			{UnquotedToken, "aaa你好bbb", 2, 1},
			{NewLine, "", 2, 9},
		})
	})

	t.Run("equals-sign", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-equals-sign.conf", []wantTok{
			{UnquotedToken, "key=value", 1, 1},
			{NewLine, "", 1, 10},
		})
	})

	t.Run("leading-dashes", func(t *testing.T) {
		t.Parallel()
		checkUnquotedFile(t, "1-unquoted-string-leading-dashes.conf", []wantTok{
			{UnquotedToken, "--flag", 1, 1},
			{NewLine, "", 1, 7},
		})
	})
}

// test data testsdata/lexer_token/goodcase/1-unquoted-string.conf
//
// That file is for humans only — it is never read by the tests, so it may lag
// behind; the table below is the single source of truth. It lists the same
// inputs, in the same order, one per line.
//
// A bare token is never escaped, decoded or transformed: its Text is the input
// verbatim. All but the last five inputs are a whole line with no separator in
// them, so they yield exactly ONE value token.
func TestTokenizeUnquotedStringGoodCases(t *testing.T) {
	const configFilePlaceholder = "config.conf"

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "dot", input: `.`, want: []string{`.`}},
		{name: "equals-sign", input: `=`, want: []string{`=`}},
		{name: "tilde", input: `~`, want: []string{`~`}},
		{name: "bare-word", input: `aaa`, want: []string{`aaa`}},
		{name: "quote-at-end", input: `aaa"`, want: []string{`aaa"`}},
		{name: "quote-inside", input: `aa"a`, want: []string{`aa"a`}},
		{name: "backtick-at-end", input: "aaa`", want: []string{"aaa`"}},
		{name: "backtick-inside", input: "aa`a", want: []string{"aa`a"}},
		{name: "brace-open-at-end", input: `aaa{`, want: []string{`aaa{`}},
		{name: "brace-close-at-end", input: `aaa}`, want: []string{`aaa}`}},
		{name: "brace-pair-at-end", input: `aaa{}`, want: []string{`aaa{}`}},
		{name: "backslash-at-end", input: `aaa\`, want: []string{`aaa\`}},
		{name: "backslash-inside", input: `aa\a`, want: []string{`aa\a`}},
		{name: "port", input: `:8080`, want: []string{`:8080`}},
		{name: "ipv4", input: `1.1.1.1`, want: []string{`1.1.1.1`}},
		{name: "brace-open-alone", input: `{`, want: []string{`{`}},
		{name: "brace-open-run", input: `{{{{{`, want: []string{`{{{{{`}},
		{name: "brace-close-alone", input: `}`, want: []string{`}`}},
		{name: "brace-close-run", input: `}}}}}`, want: []string{`}}}}}`}},
		{name: "brace-empty", input: `{}`, want: []string{`{}`}},
		{name: "hash-at-end", input: `aaa#`, want: []string{`aaa#`}},
		{name: "hash-inside", input: `aaa#bbb`, want: []string{`aaa#bbb`}},
		{name: "args-placeholder", input: `{args[0]}`, want: []string{`{args[0]}`}},
		{name: "env-placeholder", input: `{$DEEPSEEK_API_KEY}`, want: []string{`{$DEEPSEEK_API_KEY}`}},
		{name: "env-empty-name", input: `{$}`, want: []string{`{$}`}},
		{name: "env-then-text", input: `{$HOME}x`, want: []string{`{$HOME}x`}},

		// The next five cannot be shown faithfully in a text file.

		// A real CR is a Token separator, not part of the token, so it merely
		// ends the bare token.
		{name: "real-cr-at-end", input: "aaa\r", want: []string{"aaa"}},
		{name: "real-cr-around", input: "\raaa\r", want: []string{"aaa"}},
		// CRLF: the CR separates, the LF is the NewLine token.
		{name: "crlf-at-end", input: "aaa\r\n", want: []string{"aaa"}},
		// A lone CR is a pure separator, so no value token is produced at all.
		{name: "lone-cr", input: "\r", want: nil},
		// `<CR>` is just four ordinary characters, not a carriage return.
		{name: "literal-angle-cr-text", input: `aaa<CR>`, want: []string{`aaa<CR>`}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokens, err := Tokenize([]byte(tc.input), configFilePlaceholder)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := valuesOf(tokens)
			if len(got) != len(tc.want) {
				t.Fatalf("expected %d value token(s) %q, got %d (%q)", len(tc.want), tc.want, len(got), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("token %d: expected %q, got %q", i, tc.want[i], got[i])
				}
			}
		})
	}
}
