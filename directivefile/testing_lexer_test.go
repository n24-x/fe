package directivefile

import (
	"testing"
)

func TestTokenizeEmptyAndComment(t *testing.T) {
	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/goodcases/0-empty.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(tokens) > 0 {
			t.Fatalf("expected no tokens, got %d", len(tokens))
		}
	})

	t.Run("only newline token", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/goodcases/0-only-newline.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, tok := range tokens {
			if tok.Type != NewLine {
				t.Fatalf("expected NewLine token, got %s", tok.Type)
			}
		}
		if len(tokens) != 2 {
			t.Fatalf("expected 2 NewLine tokens, got %d", len(tokens))
		}
	})

	t.Run("only comment", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/goodcases/0-only-comment.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// The file holds one comment line and no trailing newline, so no token is
		// produced by scanning: the only token is the NewLine synthesised at EOF,
		// which is what terminates that line.
		if len(tokens) != 1 {
			t.Fatalf("expected 1 token (the synthesised EOF NewLine), got %d", len(tokens))
		}
	})

	t.Run("normarl comment 1", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/goodcases/1-comment-1.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, tok := range tokens {
			if tok.Type != NewLine {
				t.Fatalf("expected NewLine token, got %s", tok.Type)
			}
		}
		// 3 LFs; the last comment line has no LF, but the last TOKEN is already a
		// NewLine, so nothing is synthesised.
		if len(tokens) != 3 {
			t.Fatalf("expected 3 NewLine tokens, got %d", len(tokens))
		}
	})

	t.Run("normarl comment 2", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/goodcases/1-comment-2.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, tok := range tokens {
			if tok.Type != NewLine {
				t.Fatalf("expected NewLine token, got %s", tok.Type)
			}
		}
		// 4 LFs; same as above — the last token is already a NewLine.
		if len(tokens) != 4 {
			t.Fatalf("expected 4 NewLine tokens, got %d", len(tokens))
		}
	})

	t.Run("only one comment char", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/goodcases/1-comment-3.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, tok := range tokens {
			if tok.Type != NewLine {
				t.Fatalf("expected NewLine token, got %s", tok.Type)
			}
		}
		if len(tokens) != 1 {
			t.Fatalf("expected 1 NewLine tokens, got %d", len(tokens))
		}
	})
}

func TestTokenizeLineContinuationGoodCases(t *testing.T) {
	t.Run("double-quoted string tokens with line continuation", func(t *testing.T) {
		t.Parallel()

		const path = "testsdata/lexer/goodcases/3-quoted-string-with-line-continuation.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Three double-quoted strings, plus the NewLine synthesised at EOF.
		if len(tokens) != 4 {
			t.Fatalf("expected 4 tokens (3 strings + the EOF NewLine), got %d", len(tokens))
		}
	})
}

func TestTokenizeLineContinuationBadCases(t *testing.T) {
	t.Run("three_backslashes_are_not_line_continuations", func(t *testing.T) {
		t.Parallel()
		const path = "testsdata/lexer/badcases/5-line-continuation-9.conf"
		tokens, err := Tokenize(readTestdata(t, path), path)
		if err != nil {
			t.Fatal("expected nil, got error")
		}
		// 3 `\` + the 3 NewLines that follow them + the comment's NewLine + the
		// NewLine synthesised at EOF = 7.
		if len(tokens) != 7 {
			t.Fatalf("expected 7 tokens, got %d", len(tokens))
		}
	})
}
