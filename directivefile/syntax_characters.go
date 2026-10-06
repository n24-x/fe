package directivefile

const (
	CR            = '\r'
	LF            = '\n'
	DoubleQuote   = '"'
	BacktickQuote = '`'
	BackSlash     = '\\'
	LBRACE        = '{'
	RBRACE        = '}'
	HashTag       = '#'
)

// isASCIILetter reports whether c is an ASCII letter, i.e. a-z or A-Z.
// Deliberately ASCII-only: directive and environment-variable names are part
// of the on-disk syntax, so a look-alike Unicode letter must not be accepted.
func isASCIILetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// isASCIIDigit reports whether c is an ASCII digit, i.e. 0-9.
func isASCIIDigit(c byte) bool {
	return '0' <= c && c <= '9'
}
