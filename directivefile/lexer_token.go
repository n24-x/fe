package directivefile

import (
	"fmt"
	"unicode"
)

// Token represents a single parsable unit.
type Token struct {
	Type     TokenType // enclosing quote character, if any
	Text     string
	Position Position // start of this token — a pure rune-based location
}

type TokenType uint8

const (
	UnquotedToken       TokenType = iota // bare
	DoubleQuotedToken                    // "..."
	BacktickQuotedToken                  // `...`
	NewLine
)

func (t TokenType) String() string {
	switch t {
	case UnquotedToken:
		return "UnquotedToken"
	case DoubleQuotedToken:
		return "DoubleQuotedToken"
	case BacktickQuotedToken:
		return "BacktickQuotedToken"
	case NewLine:
		return "NewLine"
	default:
		return fmt.Sprintf("TokenType(%d)", uint8(t))
	}
}

type Position struct {
	Filename string // filename, if any
	Line     int    // line number, starting at 1
	Column   int    // column number, starting at 1 (character count per line)
}

func (pos Position) IsValid() bool {
	return pos.Line > 0 && pos.Column > 0
}

func (pos Position) String() string {
	s := pos.Filename
	if s == "" {
		s = "<input>"
	}
	if pos.IsValid() {
		s += fmt.Sprintf(":%d:%d", pos.Line, pos.Column)
	}
	return s
}

// IsTokenSeparator reports whether r separates tokens.
// LF is excluded because it is a significant token in the grammar.
func IsTokenSeparator(r rune) bool {
	if r == LF {
		return false
	}
	return unicode.IsSpace(r)
}
