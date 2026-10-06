package directivefile

import (
	"fmt"
	"strings"
)

// valuesOf returns the Text of every token that carries a value, skipping the
// structural markers. NewLine is the only one today; it is skipped because
// Tokenize always appends one at EOF (wiki/token_rules.md §EOF), which would
// otherwise add noise to every value assertion.
func valuesOf(tokens []Token) []string {
	out := make([]string, 0, len(tokens))
	for _, tk := range tokens {
		if tk.Type == NewLine {
			continue
		}
		out = append(out, tk.Text)
	}
	return out
}

// describeTokens renders a whole token stream on one line, for failure
// messages: `Type("Text")@line:col`, comma separated.
func describeTokens(tokens []Token) string {
	if len(tokens) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for i, tk := range tokens {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s(%q)@%d:%d", tk.Type, tk.Text, tk.Position.Line, tk.Position.Column)
	}
	return b.String()
}
